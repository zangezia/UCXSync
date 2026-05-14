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

type EADRecord struct {
	ProjectName         string
	RelativePath        string
	CaptureNumber       string
	EADProjectName      string
	Area                string
	RecordGUID          string
	SessionID           string
	LineNumber          int
	SegmentNumber       int
	WaypointNumber      int
	ExposureNumber      int
	CapturedAt          time.Time
	Latitude            float64
	Longitude           float64
	Altitude            float64
	AboveGroundLevel    *float64
	TrackOverGround     float64
	GroundSpeed         float64
	Software            string
	Aperture            string
	ExposureTimeSeconds float64
}

type EADProcessingStatus struct {
	ProjectName    string
	RelativePath   string
	FileSize       int64
	ModTime        time.Time
	Status         string
	WarningMessage string
	ErrorMessage   string
	ProcessedAt    time.Time
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
		`CREATE TABLE IF NOT EXISTS ead_records (
			project_name TEXT NOT NULL CHECK(TRIM(project_name) <> ''),
			relative_path TEXT NOT NULL CHECK(TRIM(relative_path) <> ''),
			capture_number TEXT NOT NULL,
			ead_project_name TEXT NOT NULL,
			area TEXT NOT NULL DEFAULT '',
			record_guid TEXT NOT NULL,
			session_id TEXT NOT NULL,
			line_number INTEGER NOT NULL,
			segment_number INTEGER NOT NULL,
			waypoint_number INTEGER NOT NULL,
			exposure_number INTEGER NOT NULL,
			captured_at TEXT NOT NULL,
			latitude REAL NOT NULL,
			longitude REAL NOT NULL,
			altitude REAL NOT NULL,
			above_ground_level REAL,
			track_over_ground REAL NOT NULL,
			ground_speed REAL NOT NULL,
			software TEXT NOT NULL DEFAULT '',
			aperture TEXT NOT NULL DEFAULT '',
			exposure_time_seconds REAL NOT NULL DEFAULT 0,
			updated_at TEXT NOT NULL,
			PRIMARY KEY(project_name, relative_path)
		);`,
		`CREATE TABLE IF NOT EXISTS ead_processing_status (
			project_name TEXT NOT NULL CHECK(TRIM(project_name) <> ''),
			relative_path TEXT NOT NULL CHECK(TRIM(relative_path) <> ''),
			file_size INTEGER NOT NULL DEFAULT 0,
			mod_time_unix_ns INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL,
			warning_message TEXT NOT NULL DEFAULT '',
			error_message TEXT NOT NULL DEFAULT '',
			processed_at TEXT NOT NULL,
			PRIMARY KEY(project_name, relative_path)
		);`,
	}

	for _, stmt := range ddl {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("failed to initialize sqlite schema: %w", err)
		}
	}

	if err := s.ensureColumnExists("ead_records", "area", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}

	return s.ensureStatusRow()
}

func (s *Store) ensureColumnExists(tableName, columnName, columnDefinition string) error {
	rows, err := s.db.Query(fmt.Sprintf(`PRAGMA table_info(%s)`, tableName))
	if err != nil {
		return fmt.Errorf("failed to inspect sqlite schema for %s: %w", tableName, err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid      int
			name     string
			colType  string
			notNull  int
			defaultV sql.NullString
			pk       int
		)
		if err := rows.Scan(&cid, &name, &colType, &notNull, &defaultV, &pk); err != nil {
			return fmt.Errorf("failed to scan sqlite schema for %s: %w", tableName, err)
		}
		if strings.EqualFold(name, columnName) {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("failed to iterate sqlite schema for %s: %w", tableName, err)
	}

	statement := fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s %s`, tableName, columnName, columnDefinition)
	if _, err := s.db.Exec(statement); err != nil {
		return fmt.Errorf("failed to add sqlite column %s.%s: %w", tableName, columnName, err)
	}

	return nil
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

func (s *Store) ListProjectDatabaseSummaries() ([]models.ProjectDatabaseSummary, error) {
	rows, err := s.db.Query(`
		WITH project_names AS (
			SELECT DISTINCT project_name FROM projects
			UNION SELECT DISTINCT project_name FROM captures
			UNION SELECT DISTINCT project_name FROM copied_files
			UNION SELECT DISTINCT project_name FROM ead_records
			UNION SELECT DISTINCT project_name FROM ead_processing_status
		)
		SELECT
			p.project_name,
			COALESCE((SELECT GROUP_CONCAT(source, ', ') FROM (
				SELECT DISTINCT source FROM projects WHERE project_name = p.project_name AND source <> '' ORDER BY source
			)), ''),
			COALESCE((SELECT MAX(last_seen_at) FROM projects WHERE project_name = p.project_name), ''),
			COALESCE((SELECT COUNT(*) FROM captures WHERE service_name = ? AND project_name = p.project_name), 0),
			COALESCE((SELECT COUNT(*) FROM captures WHERE service_name = ? AND project_name = p.project_name AND completed = 1 AND is_test = 0), 0),
			COALESCE((SELECT COUNT(*) FROM captures WHERE service_name = ? AND project_name = p.project_name AND completed = 1 AND is_test = 1), 0),
			COALESCE((SELECT COUNT(*) FROM copied_files WHERE project_name = p.project_name), 0),
			COALESCE((SELECT COUNT(*) FROM ead_records WHERE project_name = p.project_name), 0),
			COALESCE((SELECT COUNT(*) FROM ead_processing_status WHERE project_name = p.project_name), 0),
			COALESCE((SELECT capture_number FROM captures WHERE service_name = ? AND project_name = p.project_name AND completed = 1 AND is_test = 0 ORDER BY CAST(capture_number AS INTEGER) DESC, capture_number DESC LIMIT 1), ''),
			COALESCE((SELECT capture_number FROM captures WHERE service_name = ? AND project_name = p.project_name AND completed = 1 AND is_test = 1 ORDER BY CAST(capture_number AS INTEGER) DESC, capture_number DESC LIMIT 1), '')
		FROM project_names p
		WHERE TRIM(p.project_name) <> ''
		ORDER BY p.project_name ASC
	`, aggregateCaptureServiceName, aggregateCaptureServiceName, aggregateCaptureServiceName, aggregateCaptureServiceName, aggregateCaptureServiceName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	summaries := make([]models.ProjectDatabaseSummary, 0)
	for rows.Next() {
		var summary models.ProjectDatabaseSummary
		if err := rows.Scan(
			&summary.Name,
			&summary.Source,
			&summary.LastSeenAt,
			&summary.Captures,
			&summary.CompletedCaptures,
			&summary.CompletedTestCaptures,
			&summary.CopiedFiles,
			&summary.EADRecords,
			&summary.EADProcessingRecords,
			&summary.LastCaptureNumber,
			&summary.LastTestCaptureNumber,
		); err != nil {
			return nil, err
		}
		summaries = append(summaries, summary)
	}

	return summaries, rows.Err()
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

// ClearProjectHistory removes all download history for the given project:
// copied_files records, capture tracking data, and resets sync_status counters.
func (s *Store) ClearProjectHistory(project string) error {
	if strings.TrimSpace(project) == "" {
		return nil
	}

	return s.withWriteTx(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`DELETE FROM copied_files WHERE project_name = ?`, project); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM capture_files WHERE service_name = ? AND project_name = ?`, aggregateCaptureServiceName, project); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM captures WHERE service_name = ? AND project_name = ?`, aggregateCaptureServiceName, project); err != nil {
			return err
		}
		_, err := tx.Exec(`
			UPDATE sync_status
			SET completed_captures = 0, completed_test_captures = 0,
			    last_capture_number = '', last_test_capture_number = ''
			WHERE project = ?
		`, project)
		return err
	})
}

func (s *Store) DeleteProject(project string) error {
	project = strings.TrimSpace(project)
	if project == "" {
		return nil
	}

	return s.withWriteTx(func(tx *sql.Tx) error {
		var running int
		if err := tx.QueryRow(`
			SELECT COUNT(*) FROM sync_status
			WHERE is_running = 1 AND project = ?
		`, project).Scan(&running); err != nil {
			return err
		}
		if running > 0 {
			return fmt.Errorf("cannot delete project while sync is running")
		}

		if _, err := tx.Exec(`DELETE FROM projects WHERE project_name = ?`, project); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM copied_files WHERE project_name = ?`, project); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM capture_files WHERE project_name = ?`, project); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM captures WHERE project_name = ?`, project); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM ead_records WHERE project_name = ?`, project); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM ead_processing_status WHERE project_name = ?`, project); err != nil {
			return err
		}
		_, err := tx.Exec(`
			UPDATE sync_status
			SET project = '', destination = '', max_parallelism = 0,
			    completed_captures = 0, completed_test_captures = 0,
			    last_capture_number = '', last_test_capture_number = '',
			    updated_at = ?
			WHERE project = ?
		`, time.Now().UTC().Format(time.RFC3339Nano), project)
		return err
	})
}

func (s *Store) ClearDatabase() error {
	return s.withWriteTx(func(tx *sql.Tx) error {
		var running int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM sync_status WHERE is_running = 1`).Scan(&running); err != nil {
			return err
		}
		if running > 0 {
			return fmt.Errorf("cannot clear database while sync is running")
		}

		for _, query := range []string{
			`DELETE FROM projects`,
			`DELETE FROM copied_files`,
			`DELETE FROM capture_files`,
			`DELETE FROM captures`,
			`DELETE FROM ead_records`,
			`DELETE FROM ead_processing_status`,
		} {
			if _, err := tx.Exec(query); err != nil {
				return err
			}
		}

		_, err := tx.Exec(`
			UPDATE sync_status
			SET is_running = 0, project = '', destination = '', max_parallelism = 0,
			    completed_captures = 0, completed_test_captures = 0,
			    last_capture_number = '', last_test_capture_number = '',
			    updated_at = ?
		`, time.Now().UTC().Format(time.RFC3339Nano))
		return err
	})
}

// IsCaptureDone reports whether the given capture has been marked completed
// in the aggregate captures table. Returns false if the capture is unknown.
func (s *Store) IsCaptureDone(project, captureNumber string) (bool, error) {
	if strings.TrimSpace(project) == "" || strings.TrimSpace(captureNumber) == "" {
		return false, nil
	}

	var completed int
	err := s.db.QueryRow(`
		SELECT COALESCE(completed, 0)
		FROM captures
		WHERE service_name = ? AND project_name = ? AND capture_number = ?
	`, aggregateCaptureServiceName, project, captureNumber).Scan(&completed)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	return completed > 0, nil
}

// ResetProjectCaptureStatus resets the completion state of all captures for
// the given project: clears capture_files records and sets completed=0 so
// the counters start rebuilding from scratch on the next sync run.
func (s *Store) ResetProjectCaptureStatus(project string) error {
	if strings.TrimSpace(project) == "" {
		return nil
	}

	return s.withWriteTx(func(tx *sql.Tx) error {
		_, err := tx.Exec(`
			DELETE FROM capture_files
			WHERE service_name = ? AND project_name = ?
		`, aggregateCaptureServiceName, project)
		if err != nil {
			return err
		}

		_, err = tx.Exec(`
			UPDATE captures
			SET completed = 0, completed_at = NULL, raw_count = 0, has_xml = 0, has_dat = 0
			WHERE service_name = ? AND project_name = ?
		`, aggregateCaptureServiceName, project)
		return err
	})
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

		rawCount, hasXML, hasDAT, isTest, alreadyCompleted, err := s.captureProgress(tx, obs.Project, obs.Info.CaptureNumber)
		if err != nil {
			return err
		}

		completed := false
		requireXML := obs.RequireXML && !isTest
		requireDAT := obs.RequireDAT && !isTest
		shouldComplete := rawCount == obs.RequiredRawFiles && (!requireXML || hasXML) && (!requireDAT || hasDAT)
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

func (s *Store) SaveEADProcessing(record EADRecord, processing EADProcessingStatus) error {
	return s.withWriteTx(func(tx *sql.Tx) error {
		processedAt := processing.ProcessedAt.UTC()
		if processedAt.IsZero() {
			processedAt = time.Now().UTC()
		}

		updatedAt := time.Now().UTC().Format(time.RFC3339Nano)
		capturedAt := record.CapturedAt.UTC().Format(time.RFC3339Nano)

		var aboveGroundLevel any
		if record.AboveGroundLevel != nil {
			aboveGroundLevel = *record.AboveGroundLevel
		}

		_, err := tx.Exec(`
			INSERT INTO ead_records (
				project_name, relative_path, capture_number, ead_project_name,
				area, record_guid, session_id, line_number, segment_number, waypoint_number,
				exposure_number, captured_at, latitude, longitude, altitude,
				above_ground_level, track_over_ground, ground_speed, software,
				aperture, exposure_time_seconds, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(project_name, relative_path)
			DO UPDATE SET
				capture_number = excluded.capture_number,
				ead_project_name = excluded.ead_project_name,
				area = excluded.area,
				record_guid = excluded.record_guid,
				session_id = excluded.session_id,
				line_number = excluded.line_number,
				segment_number = excluded.segment_number,
				waypoint_number = excluded.waypoint_number,
				exposure_number = excluded.exposure_number,
				captured_at = excluded.captured_at,
				latitude = excluded.latitude,
				longitude = excluded.longitude,
				altitude = excluded.altitude,
				above_ground_level = excluded.above_ground_level,
				track_over_ground = excluded.track_over_ground,
				ground_speed = excluded.ground_speed,
				software = excluded.software,
				aperture = excluded.aperture,
				exposure_time_seconds = excluded.exposure_time_seconds,
				updated_at = excluded.updated_at
		`,
			record.ProjectName,
			normalizeRelativePath(record.RelativePath),
			record.CaptureNumber,
			record.EADProjectName,
			record.Area,
			record.RecordGUID,
			record.SessionID,
			record.LineNumber,
			record.SegmentNumber,
			record.WaypointNumber,
			record.ExposureNumber,
			capturedAt,
			record.Latitude,
			record.Longitude,
			record.Altitude,
			aboveGroundLevel,
			record.TrackOverGround,
			record.GroundSpeed,
			record.Software,
			record.Aperture,
			record.ExposureTimeSeconds,
			updatedAt,
		)
		if err != nil {
			return err
		}

		_, err = tx.Exec(`
			INSERT INTO ead_processing_status (
				project_name, relative_path, file_size, mod_time_unix_ns,
				status, warning_message, error_message, processed_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(project_name, relative_path)
			DO UPDATE SET
				file_size = excluded.file_size,
				mod_time_unix_ns = excluded.mod_time_unix_ns,
				status = excluded.status,
				warning_message = excluded.warning_message,
				error_message = excluded.error_message,
				processed_at = excluded.processed_at
		`,
			processing.ProjectName,
			normalizeRelativePath(processing.RelativePath),
			processing.FileSize,
			processing.ModTime.UTC().UnixNano(),
			normalizeEADProcessingState(processing.Status),
			strings.TrimSpace(processing.WarningMessage),
			strings.TrimSpace(processing.ErrorMessage),
			processedAt.Format(time.RFC3339Nano),
		)
		return err
	})
}

func (s *Store) RecordEADProcessingFailure(processing EADProcessingStatus) error {
	processedAt := processing.ProcessedAt.UTC()
	if processedAt.IsZero() {
		processedAt = time.Now().UTC()
	}

	return s.withWriteTx(func(tx *sql.Tx) error {
		project := strings.TrimSpace(processing.ProjectName)
		relativePath := normalizeRelativePath(processing.RelativePath)

		if _, err := tx.Exec(`
			DELETE FROM ead_records
			WHERE project_name = ? AND relative_path = ?
		`, project, relativePath); err != nil {
			return err
		}

		_, err := tx.Exec(`
			INSERT INTO ead_processing_status (
				project_name, relative_path, file_size, mod_time_unix_ns,
				status, warning_message, error_message, processed_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(project_name, relative_path)
			DO UPDATE SET
				file_size = excluded.file_size,
				mod_time_unix_ns = excluded.mod_time_unix_ns,
				status = excluded.status,
				warning_message = excluded.warning_message,
				error_message = excluded.error_message,
				processed_at = excluded.processed_at
		`,
			project,
			relativePath,
			processing.FileSize,
			processing.ModTime.UTC().UnixNano(),
			normalizeEADProcessingState(processing.Status),
			strings.TrimSpace(processing.WarningMessage),
			strings.TrimSpace(processing.ErrorMessage),
			processedAt.Format(time.RFC3339Nano),
		)
		return err
	})
}

func (s *Store) LoadEADRecord(project, relativePath string) (EADRecord, bool, error) {
	var (
		record           EADRecord
		capturedAtRaw    string
		aboveGroundLevel sql.NullFloat64
	)

	err := s.db.QueryRow(`
		SELECT
			project_name, relative_path, capture_number, ead_project_name,
			area, record_guid, session_id, line_number, segment_number, waypoint_number,
			exposure_number, captured_at, latitude, longitude, altitude,
			above_ground_level, track_over_ground, ground_speed, software,
			aperture, exposure_time_seconds
		FROM ead_records
		WHERE project_name = ? AND relative_path = ?
	`, project, normalizeRelativePath(relativePath)).Scan(
		&record.ProjectName,
		&record.RelativePath,
		&record.CaptureNumber,
		&record.EADProjectName,
		&record.Area,
		&record.RecordGUID,
		&record.SessionID,
		&record.LineNumber,
		&record.SegmentNumber,
		&record.WaypointNumber,
		&record.ExposureNumber,
		&capturedAtRaw,
		&record.Latitude,
		&record.Longitude,
		&record.Altitude,
		&aboveGroundLevel,
		&record.TrackOverGround,
		&record.GroundSpeed,
		&record.Software,
		&record.Aperture,
		&record.ExposureTimeSeconds,
	)
	if err == sql.ErrNoRows {
		return EADRecord{}, false, nil
	}
	if err != nil {
		return EADRecord{}, false, err
	}

	if capturedAtRaw != "" {
		record.CapturedAt, err = time.Parse(time.RFC3339Nano, capturedAtRaw)
		if err != nil {
			return EADRecord{}, false, err
		}
	}
	if aboveGroundLevel.Valid {
		record.AboveGroundLevel = &aboveGroundLevel.Float64
	}

	return record, true, nil
}

func (s *Store) LoadEADProcessingStatus(project, relativePath string) (EADProcessingStatus, bool, error) {
	var (
		status         EADProcessingStatus
		processedAtRaw string
		modTimeUnixNS  int64
	)

	err := s.db.QueryRow(`
		SELECT
			project_name, relative_path, file_size, mod_time_unix_ns,
			status, warning_message, error_message, processed_at
		FROM ead_processing_status
		WHERE project_name = ? AND relative_path = ?
	`, project, normalizeRelativePath(relativePath)).Scan(
		&status.ProjectName,
		&status.RelativePath,
		&status.FileSize,
		&modTimeUnixNS,
		&status.Status,
		&status.WarningMessage,
		&status.ErrorMessage,
		&processedAtRaw,
	)
	if err == sql.ErrNoRows {
		return EADProcessingStatus{}, false, nil
	}
	if err != nil {
		return EADProcessingStatus{}, false, err
	}

	status.ModTime = time.Unix(0, modTimeUnixNS).UTC()
	if processedAtRaw != "" {
		status.ProcessedAt, err = time.Parse(time.RFC3339Nano, processedAtRaw)
		if err != nil {
			return EADProcessingStatus{}, false, err
		}
	}

	return status, true, nil
}

func (s *Store) ListCompletedEADRecords(project string) ([]EADRecord, error) {
	if strings.TrimSpace(project) == "" {
		return nil, nil
	}

	rows, err := s.db.Query(`
		SELECT
			e.project_name, e.relative_path, e.capture_number, e.ead_project_name,
			e.area, e.record_guid, e.session_id, e.line_number, e.segment_number,
			e.waypoint_number, e.exposure_number, e.captured_at, e.latitude,
			e.longitude, e.altitude, e.above_ground_level, e.track_over_ground,
			e.ground_speed, e.software, e.aperture, e.exposure_time_seconds
		FROM ead_records e
		INNER JOIN captures c
			ON c.service_name = ?
			AND c.project_name = e.project_name
			AND c.capture_number = e.capture_number
		INNER JOIN ead_processing_status eps
			ON eps.project_name = e.project_name
			AND eps.relative_path = e.relative_path
		WHERE e.project_name = ? AND c.completed = 1 AND eps.status = 'success'
		ORDER BY CAST(e.capture_number AS INTEGER) ASC, e.line_number ASC, e.waypoint_number ASC, e.exposure_number ASC
	`, aggregateCaptureServiceName, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]EADRecord, 0)
	for rows.Next() {
		var (
			record        EADRecord
			capturedAtRaw string
			agl           sql.NullFloat64
		)
		if err := rows.Scan(
			&record.ProjectName,
			&record.RelativePath,
			&record.CaptureNumber,
			&record.EADProjectName,
			&record.Area,
			&record.RecordGUID,
			&record.SessionID,
			&record.LineNumber,
			&record.SegmentNumber,
			&record.WaypointNumber,
			&record.ExposureNumber,
			&capturedAtRaw,
			&record.Latitude,
			&record.Longitude,
			&record.Altitude,
			&agl,
			&record.TrackOverGround,
			&record.GroundSpeed,
			&record.Software,
			&record.Aperture,
			&record.ExposureTimeSeconds,
		); err != nil {
			return nil, err
		}
		if capturedAtRaw != "" {
			record.CapturedAt, err = time.Parse(time.RFC3339Nano, capturedAtRaw)
			if err != nil {
				return nil, err
			}
		}
		if agl.Valid {
			record.AboveGroundLevel = &agl.Float64
		}
		records = append(records, record)
	}

	return records, rows.Err()
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

func (s *Store) captureProgress(tx *sql.Tx, project, captureNumber string) (rawCount int, hasXML bool, hasDAT bool, isTest bool, completed bool, err error) {
	var hasXMLInt, hasDATInt, isTestInt, completedInt int
	aggregateService := aggregateCaptureServiceName
	err = tx.QueryRow(`
		SELECT
			(SELECT COUNT(*) FROM capture_files WHERE service_name = ? AND project_name = ? AND capture_number = ? AND file_key GLOB 'raw:[0-9][0-9]-[0-9][0-9]') AS raw_count,
			(SELECT COUNT(*) FROM capture_files WHERE service_name = ? AND project_name = ? AND capture_number = ? AND file_key = 'xml:CU') AS has_xml,
			(SELECT COUNT(*) FROM capture_files WHERE service_name = ? AND project_name = ? AND capture_number = ? AND file_key = 'dat:CU') AS has_dat,
			COALESCE((SELECT is_test FROM captures WHERE service_name = ? AND project_name = ? AND capture_number = ?), 0) AS is_test,
			COALESCE((SELECT completed FROM captures WHERE service_name = ? AND project_name = ? AND capture_number = ?), 0) AS completed
	`, aggregateService, project, captureNumber, aggregateService, project, captureNumber, aggregateService, project, captureNumber, aggregateService, project, captureNumber, aggregateService, project, captureNumber).Scan(&rawCount, &hasXMLInt, &hasDATInt, &isTestInt, &completedInt)
	return rawCount, hasXMLInt > 0, hasDATInt > 0, isTestInt > 0, completedInt > 0, err
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
		ORDER BY CAST(capture_number AS INTEGER) DESC, capture_number DESC
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
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	return filepath.ToSlash(filepath.Clean(path))
}

func normalizeEADProcessingState(state string) string {
	state = strings.ToLower(strings.TrimSpace(state))
	if state == "" {
		return "success"
	}
	return state
}

func SortProjects(projects []models.ProjectInfo) {
	sort.Slice(projects, func(i, j int) bool {
		if projects[i].Name == projects[j].Name {
			return projects[i].Source < projects[j].Source
		}
		return projects[i].Name < projects[j].Name
	})
}
