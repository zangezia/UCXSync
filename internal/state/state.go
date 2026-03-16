package state

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"

	"github.com/zangezia/UCXSync/pkg/models"
)

type Store struct {
	db          *sql.DB
	serviceName string
	writeMu     sync.Mutex
}

const aggregateCaptureServiceName = "__capture_aggregate__"

const (
	busyRetryCount = 8
	busyRetryDelay = 100 * time.Millisecond
)

type StatusSnapshot struct {
	IsRunning             bool
	Project               string
	Destination           string
	MaxParallelism        int
	CompletedCaptures     int
	CompletedTestCaptures int
	LastCaptureNumber     string
	LastTestCaptureNumber string
}

type CaptureObservation struct {
	Project          string
	Info             models.CaptureInfo
	FileKey          string
	RequiredRawFiles int
	RequireXML       bool
	RequireDAT       bool
}

func New(path, serviceName string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("sqlite path must not be empty")
	}
	if strings.TrimSpace(serviceName) == "" {
		serviceName = "ucxsync"
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("failed to create sqlite directory: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)
	db.SetConnMaxIdleTime(0)

	store := &Store{db: db, serviceName: serviceName}
	if err := store.init(); err != nil {
		db.Close()
		return nil, err
	}

	return store, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) init() error {
	pragmas := []string{
		"PRAGMA journal_mode=WAL;",
		"PRAGMA synchronous=NORMAL;",
		"PRAGMA busy_timeout=5000;",
		"PRAGMA foreign_keys=ON;",
	}
	for _, pragma := range pragmas {
		if _, err := s.db.Exec(pragma); err != nil {
			return fmt.Errorf("failed to apply sqlite pragma %q: %w", pragma, err)
		}
	}

	ddl := []string{
		`CREATE TABLE IF NOT EXISTS sync_status (
			service_name TEXT PRIMARY KEY,
			is_running INTEGER NOT NULL DEFAULT 0,
			project TEXT NOT NULL DEFAULT '',
			destination TEXT NOT NULL DEFAULT '',
			max_parallelism INTEGER NOT NULL DEFAULT 0,
			completed_captures INTEGER NOT NULL DEFAULT 0,
			completed_test_captures INTEGER NOT NULL DEFAULT 0,
			last_capture_number TEXT NOT NULL DEFAULT '',
			last_test_capture_number TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS projects (
			service_name TEXT NOT NULL,
			project_name TEXT NOT NULL,
			source TEXT NOT NULL,
			last_seen_at TEXT NOT NULL,
			PRIMARY KEY(service_name, project_name)
		);`,
		`CREATE TABLE IF NOT EXISTS captures (
			service_name TEXT NOT NULL,
			project_name TEXT NOT NULL,
			capture_number TEXT NOT NULL,
			is_test INTEGER NOT NULL DEFAULT 0,
			data_type TEXT NOT NULL DEFAULT '',
			sensor_code TEXT NOT NULL DEFAULT '',
			session_id TEXT NOT NULL DEFAULT '',
			is_verified INTEGER NOT NULL DEFAULT 0,
			raw_count INTEGER NOT NULL DEFAULT 0,
			has_xml INTEGER NOT NULL DEFAULT 0,
			has_dat INTEGER NOT NULL DEFAULT 0,
			completed INTEGER NOT NULL DEFAULT 0,
			completed_at TEXT,
			last_seen_at TEXT NOT NULL,
			PRIMARY KEY(service_name, project_name, capture_number)
		);`,
		`CREATE TABLE IF NOT EXISTS capture_files (
			service_name TEXT NOT NULL,
			project_name TEXT NOT NULL,
			capture_number TEXT NOT NULL,
			file_key TEXT NOT NULL,
			PRIMARY KEY(service_name, project_name, capture_number, file_key),
			FOREIGN KEY(service_name, project_name, capture_number)
				REFERENCES captures(service_name, project_name, capture_number)
				ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS copied_files (
			project_name TEXT NOT NULL,
			relative_path TEXT NOT NULL,
			file_size INTEGER NOT NULL DEFAULT 0,
			mod_time_unix_ns INTEGER NOT NULL DEFAULT 0,
			copied_at TEXT NOT NULL,
			PRIMARY KEY(project_name, relative_path)
		);`,
	}

	for _, stmt := range ddl {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("failed to initialize sqlite schema: %w", err)
		}
	}

	return s.ensureStatusRow()
}

func (s *Store) ensureStatusRow() error {
	return s.execWrite(`
		INSERT INTO sync_status (service_name)
		VALUES (?)
		ON CONFLICT(service_name) DO NOTHING
	`, s.serviceName)
}

func (s *Store) LoadStatus() (StatusSnapshot, error) {
	if err := s.ensureStatusRow(); err != nil {
		return StatusSnapshot{}, err
	}

	var snapshot StatusSnapshot
	err := s.db.QueryRow(`
		SELECT is_running, project, destination, max_parallelism,
		       completed_captures, completed_test_captures,
		       last_capture_number, last_test_capture_number
		FROM sync_status
		WHERE service_name = ?
	`, s.serviceName).Scan(
		&snapshot.IsRunning,
		&snapshot.Project,
		&snapshot.Destination,
		&snapshot.MaxParallelism,
		&snapshot.CompletedCaptures,
		&snapshot.CompletedTestCaptures,
		&snapshot.LastCaptureNumber,
		&snapshot.LastTestCaptureNumber,
	)
	if err != nil {
		return StatusSnapshot{}, err
	}

	return snapshot, nil
}

func (s *Store) StartRun(project, destination string, maxParallelism int) (StatusSnapshot, error) {
	if err := s.ensureStatusRow(); err != nil {
		return StatusSnapshot{}, err
	}

	stats, err := s.projectStats(project)
	if err != nil {
		return StatusSnapshot{}, err
	}

	err = s.execWrite(`
		UPDATE sync_status
		SET is_running = 1,
		    project = ?,
		    destination = ?,
		    max_parallelism = ?,
		    completed_captures = ?,
		    completed_test_captures = ?,
		    last_capture_number = ?,
		    last_test_capture_number = ?,
		    updated_at = ?
		WHERE service_name = ?
	`, project, destination, maxParallelism, stats.CompletedCaptures, stats.CompletedTestCaptures, stats.LastCaptureNumber, stats.LastTestCaptureNumber, time.Now().UTC().Format(time.RFC3339Nano), s.serviceName)
	if err != nil {
		return StatusSnapshot{}, err
	}

	stats.IsRunning = true
	stats.Project = project
	stats.Destination = destination
	stats.MaxParallelism = maxParallelism
	return stats, nil
}

func (s *Store) StopRun(snapshot StatusSnapshot) error {
	if err := s.ensureStatusRow(); err != nil {
		return err
	}

	err := s.execWrite(`
		UPDATE sync_status
		SET is_running = 0,
		    project = ?,
		    destination = ?,
		    max_parallelism = ?,
		    completed_captures = ?,
		    completed_test_captures = ?,
		    last_capture_number = ?,
		    last_test_capture_number = ?,
		    updated_at = ?
		WHERE service_name = ?
	`, snapshot.Project, snapshot.Destination, snapshot.MaxParallelism, snapshot.CompletedCaptures, snapshot.CompletedTestCaptures, snapshot.LastCaptureNumber, snapshot.LastTestCaptureNumber, time.Now().UTC().Format(time.RFC3339Nano), s.serviceName)
	return err
}

func (s *Store) SaveProjects(projects []models.ProjectInfo) error {
	if err := s.ensureStatusRow(); err != nil {
		return err
	}

	if len(projects) == 0 {
		return nil
	}

	return s.withWriteTx(func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		stmt, err := tx.Prepare(`
			INSERT INTO projects (service_name, project_name, source, last_seen_at)
			VALUES (?, ?, ?, ?)
			ON CONFLICT(service_name, project_name)
			DO UPDATE SET source = excluded.source, last_seen_at = excluded.last_seen_at
		`)
		if err != nil {
			return err
		}
		defer stmt.Close()

		for _, project := range projects {
			if _, err := stmt.Exec(s.serviceName, project.Name, project.Source, now); err != nil {
				return err
			}
		}

		return nil
	})
}

func (s *Store) LoadProjects() ([]models.ProjectInfo, error) {
	rows, err := s.db.Query(`
		SELECT project_name, source
		FROM projects
		WHERE service_name = ?
		ORDER BY project_name ASC
	`, s.serviceName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	projects := make([]models.ProjectInfo, 0)
	for rows.Next() {
		var project models.ProjectInfo
		if err := rows.Scan(&project.Name, &project.Source); err != nil {
			return nil, err
		}
		projects = append(projects, project)
	}

	return projects, rows.Err()
}

func (s *Store) IsFileCopied(project, relativePath string, fileSize int64, modTime time.Time) (bool, error) {
	if strings.TrimSpace(project) == "" || strings.TrimSpace(relativePath) == "" {
		return false, nil
	}

	var storedSize int64
	var storedModTime int64
	err := s.db.QueryRow(`
		SELECT file_size, mod_time_unix_ns
		FROM copied_files
		WHERE project_name = ? AND relative_path = ?
	`, project, normalizeRelativePath(relativePath)).Scan(&storedSize, &storedModTime)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	return storedSize == fileSize && storedModTime == modTime.UTC().UnixNano(), nil
}

func (s *Store) MarkFileCopied(project, relativePath string, fileSize int64, modTime time.Time) error {
	if strings.TrimSpace(project) == "" || strings.TrimSpace(relativePath) == "" {
		return nil
	}

	relativePath = normalizeRelativePath(relativePath)
	return s.execWrite(`
		INSERT INTO copied_files (project_name, relative_path, file_size, mod_time_unix_ns, copied_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(project_name, relative_path)
		DO UPDATE SET
			file_size = excluded.file_size,
			mod_time_unix_ns = excluded.mod_time_unix_ns,
			copied_at = excluded.copied_at
	`, project, relativePath, fileSize, modTime.UTC().UnixNano(), time.Now().UTC().Format(time.RFC3339Nano))
}

func (s *Store) ResetCopiedFiles(project string) error {
	if strings.TrimSpace(project) == "" {
		return nil
	}

	return s.execWrite(`
		DELETE FROM copied_files
		WHERE project_name = ?
	`, project)
}

func (s *Store) RecordCapture(obs CaptureObservation) (models.PersistedCaptureStatus, bool, error) {
	if strings.TrimSpace(obs.Project) == "" {
		return models.PersistedCaptureStatus{}, false, nil
	}

	var (
		resultStatus models.PersistedCaptureStatus
		resultDone   bool
	)

	err := s.withWriteTx(func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		aggregateService := aggregateCaptureServiceName

		_, err := tx.Exec(`
		INSERT INTO captures (
			service_name, project_name, capture_number, is_test, data_type,
			sensor_code, session_id, is_verified, last_seen_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(service_name, project_name, capture_number)
		DO UPDATE SET
			is_test = CASE WHEN captures.is_test = 1 OR excluded.is_test = 1 THEN 1 ELSE 0 END,
			data_type = excluded.data_type,
			sensor_code = excluded.sensor_code,
			session_id = excluded.session_id,
			is_verified = excluded.is_verified,
			last_seen_at = excluded.last_seen_at
	`,
			aggregateService,
			obs.Project,
			obs.Info.CaptureNumber,
			boolToInt(obs.Info.IsTest),
			obs.Info.DataType,
			obs.Info.SensorCode,
			obs.Info.SessionID,
			boolToInt(obs.Info.IsVerified),
			now,
		)
		if err != nil {
			return err
		}

		result, err := tx.Exec(`
		INSERT INTO capture_files (service_name, project_name, capture_number, file_key)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(service_name, project_name, capture_number, file_key) DO NOTHING
	`, aggregateService, obs.Project, obs.Info.CaptureNumber, obs.FileKey)
		if err != nil {
			return err
		}

		inserted := false
		if affected, err := result.RowsAffected(); err == nil {
			inserted = affected > 0
		}

		rawCount, hasXML, hasDAT, alreadyCompleted, err := s.captureProgress(tx, obs.Project, obs.Info.CaptureNumber)
		if err != nil {
			return err
		}

		completed := false
		shouldComplete := rawCount == obs.RequiredRawFiles && (!obs.RequireXML || hasXML) && (!obs.RequireDAT || hasDAT)
		if shouldComplete && !alreadyCompleted {
			completed = inserted || !alreadyCompleted
		}

		completedAt := any(nil)
		if completed {
			completedAt = now
		}

		_, err = tx.Exec(`
		UPDATE captures
		SET raw_count = ?, has_xml = ?, has_dat = ?, completed = CASE WHEN ? THEN 1 ELSE completed END,
		    completed_at = CASE WHEN ? THEN ? ELSE completed_at END,
		    last_seen_at = ?
		WHERE service_name = ? AND project_name = ? AND capture_number = ?
	`, rawCount, boolToInt(hasXML), boolToInt(hasDAT), shouldComplete, completed, completedAt, now, aggregateService, obs.Project, obs.Info.CaptureNumber)
		if err != nil {
			return err
		}

		stats, err := s.persistedCaptureStatusTx(tx, obs.Project)
		if err != nil {
			return err
		}
		stats.RawCount = rawCount
		stats.HasXML = hasXML
		stats.HasDAT = hasDAT

		_, err = tx.Exec(`
		UPDATE sync_status
		SET completed_captures = ?,
		    completed_test_captures = ?,
		    last_capture_number = ?,
		    last_test_capture_number = ?,
		    updated_at = ?
		WHERE project = ?
	`, stats.CompletedCaptures, stats.CompletedTestCaptures, stats.LastCaptureNumber, stats.LastTestCaptureNumber, now, obs.Project)
		if err != nil {
			return err
		}

		resultStatus = stats
		resultDone = completed
		return nil
	})
	if err != nil {
		return models.PersistedCaptureStatus{}, false, err
	}

	return resultStatus, resultDone, nil
}

func (s *Store) LoadProjectStatus(project string) (models.PersistedCaptureStatus, error) {
	if strings.TrimSpace(project) == "" {
		return models.PersistedCaptureStatus{}, nil
	}

	stats, err := s.projectStats(project)
	if err != nil {
		return models.PersistedCaptureStatus{}, err
	}

	return models.PersistedCaptureStatus{
		CompletedCaptures:     stats.CompletedCaptures,
		CompletedTestCaptures: stats.CompletedTestCaptures,
		LastCaptureNumber:     stats.LastCaptureNumber,
		LastTestCaptureNumber: stats.LastTestCaptureNumber,
	}, nil
}

func (s *Store) persistedCaptureStatusTx(tx *sql.Tx, project string) (models.PersistedCaptureStatus, error) {
	stats, err := s.projectStatsTx(tx, project)
	if err != nil {
		return models.PersistedCaptureStatus{}, err
	}

	return models.PersistedCaptureStatus{
		CompletedCaptures:     stats.CompletedCaptures,
		CompletedTestCaptures: stats.CompletedTestCaptures,
		LastCaptureNumber:     stats.LastCaptureNumber,
		LastTestCaptureNumber: stats.LastTestCaptureNumber,
	}, nil
}

func (s *Store) captureProgress(tx *sql.Tx, project, captureNumber string) (rawCount int, hasXML bool, hasDAT bool, completed bool, err error) {
	var hasXMLInt, hasDATInt, completedInt int
	aggregateService := aggregateCaptureServiceName
	err = tx.QueryRow(`
		SELECT
			(SELECT COUNT(*) FROM capture_files WHERE service_name = ? AND project_name = ? AND capture_number = ? AND file_key GLOB 'raw:[0-9][0-9]-[0-9][0-9]') AS raw_count,
			(SELECT COUNT(*) FROM capture_files WHERE service_name = ? AND project_name = ? AND capture_number = ? AND file_key = 'xml:CU') AS has_xml,
			(SELECT COUNT(*) FROM capture_files WHERE service_name = ? AND project_name = ? AND capture_number = ? AND file_key = 'dat:CU') AS has_dat,
			COALESCE((SELECT completed FROM captures WHERE service_name = ? AND project_name = ? AND capture_number = ?), 0) AS completed
	`, aggregateService, project, captureNumber, aggregateService, project, captureNumber, aggregateService, project, captureNumber, aggregateService, project, captureNumber).Scan(&rawCount, &hasXMLInt, &hasDATInt, &completedInt)
	return rawCount, hasXMLInt > 0, hasDATInt > 0, completedInt > 0, err
}

func (s *Store) projectStats(project string) (StatusSnapshot, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return StatusSnapshot{}, err
	}
	defer tx.Rollback()

	stats, err := s.projectStatsTx(tx, project)
	if err != nil {
		return StatusSnapshot{}, err
	}

	if err := tx.Commit(); err != nil {
		return StatusSnapshot{}, err
	}

	return stats, nil
}

func (s *Store) projectStatsTx(tx *sql.Tx, project string) (StatusSnapshot, error) {
	var stats StatusSnapshot
	stats.Project = project

	err := tx.QueryRow(`
		SELECT
			COALESCE(SUM(CASE WHEN completed = 1 AND is_test = 0 THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN completed = 1 AND is_test = 1 THEN 1 ELSE 0 END), 0)
		FROM captures
		WHERE service_name = ? AND project_name = ?
	`, aggregateCaptureServiceName, project).Scan(&stats.CompletedCaptures, &stats.CompletedTestCaptures)
	if err != nil {
		return StatusSnapshot{}, err
	}

	stats.LastCaptureNumber, err = s.latestCompletedCapture(tx, project, false)
	if err != nil {
		return StatusSnapshot{}, err
	}
	stats.LastTestCaptureNumber, err = s.latestCompletedCapture(tx, project, true)
	if err != nil {
		return StatusSnapshot{}, err
	}

	return stats, nil
}

func (s *Store) latestCompletedCapture(tx *sql.Tx, project string, isTest bool) (string, error) {
	var captureNumber sql.NullString
	err := tx.QueryRow(`
		SELECT capture_number
		FROM captures
		WHERE service_name = ? AND project_name = ? AND completed = 1 AND is_test = ?
		ORDER BY completed_at DESC, capture_number DESC
		LIMIT 1
	`, aggregateCaptureServiceName, project, boolToInt(isTest)).Scan(&captureNumber)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if !captureNumber.Valid {
		return "", nil
	}
	return captureNumber.String, nil
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func (s *Store) execWrite(query string, args ...any) error {
	return s.withWriteTx(func(tx *sql.Tx) error {
		_, err := tx.Exec(query, args...)
		return err
	})
}

func (s *Store) withWriteTx(fn func(tx *sql.Tx) error) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	var lastErr error
	for attempt := 0; attempt < busyRetryCount; attempt++ {
		tx, err := s.db.Begin()
		if err != nil {
			if isSQLiteBusyError(err) {
				lastErr = err
				time.Sleep(time.Duration(attempt+1) * busyRetryDelay)
				continue
			}
			return err
		}

		err = fn(tx)
		if err != nil {
			_ = tx.Rollback()
			if isSQLiteBusyError(err) {
				lastErr = err
				time.Sleep(time.Duration(attempt+1) * busyRetryDelay)
				continue
			}
			return err
		}

		if err := tx.Commit(); err != nil {
			_ = tx.Rollback()
			if isSQLiteBusyError(err) {
				lastErr = err
				time.Sleep(time.Duration(attempt+1) * busyRetryDelay)
				continue
			}
			return err
		}

		return nil
	}

	if lastErr != nil {
		return lastErr
	}

	return fmt.Errorf("sqlite write failed after retries")
}

func isSQLiteBusyError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "sqlite_busy") || strings.Contains(message, "database is locked") || strings.Contains(message, "database table is locked")
}

func normalizeRelativePath(path string) string {
	return filepath.ToSlash(filepath.Clean(strings.TrimSpace(path)))
}

func SortProjects(projects []models.ProjectInfo) {
	sort.Slice(projects, func(i, j int) bool {
		if projects[i].Name == projects[j].Name {
			return projects[i].Source < projects[j].Source
		}
		return projects[i].Name < projects[j].Name
	})
}
