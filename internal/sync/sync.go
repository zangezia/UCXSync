package sync

// File synchronization service for UCX capture files.
//
// Capture file naming convention:
//
// RAW files (from WU01-WU13 nodes):
//   Lvl00-00001-Arh2k_mezen_200725-06-00-BD11EBB0_BE00_4BE7_BC66_9DED8D740C2E.raw
//   Lvl0X-00002-T-Arh2k_mezen_200725-06-00-BD11EBB0_BE00_4BE7_BC66_9DED8D740C2E.raw
//
// XML metadata files (from CU node):
//   EAD-00001-Arh2k_mezen_200725-BD11EBB0_BE00_4BE7_BC66_9DED8D740C2E.xml
//
// RAW format breakdown:
//   Lvl00 or Lvl0X  - File type (Lvl00=verified, Lvl0X=unverified)
//   00001           - Capture number (5 digits)
//   T               - Test marker (optional, after capture number with dash)
//   Arh2k_mezen_200725 - Project name
//   06-00           - Sensor code (XX-YY format)
//   BD11EBB0_...    - Session GUID (unique identifier)
//   .raw            - File extension
//
// XML format breakdown:
//   EAD             - Metadata file prefix
//   00001           - Capture number (5 digits)
//   Arh2k_mezen_200725 - Project name
//   BD11EBB0_...    - Session GUID (unique identifier)
//   .xml            - File extension
//
// Complete capture requirements:
//   Normal capture: 13 RAW files (one per WU01-WU13) + 1 XML file (from CU) = 14 files total
//   Test capture:   13 RAW files (one per WU01-WU13), XML file may be missing = 13 files total

import (
	"context"
	"fmt"
	"io"
	"os"
	pathpkg "path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/zangezia/UCXSync/internal/state"
	"github.com/zangezia/UCXSync/pkg/models"
)

// Service handles file synchronization operations
type Service struct {
	nodes           []string
	shares          []string
	baseMountDir    string // Base directory for mounted shares (e.g., /ucmount)
	requiredSensors map[string]struct{}
	stateStore      *state.Store
	forceFullResync bool

	mu                    sync.RWMutex
	isRunning             bool
	project               string
	destination           string
	maxParallelism        int
	globalSemaphore       chan struct{} // Global semaphore limiting total concurrent file operations
	activeTasks           map[string]*taskInfo
	captureTracker        map[string]map[string]bool // capture# -> fileType (raw/xml) -> completed
	completedCaptures     int32
	completedTestCaptures int32
	lastCaptureNumber     string
	lastTestCaptureNumber string

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

type taskInfo struct {
	node         string
	share        string
	totalFiles   int32
	copiedFiles  int32
	failedFiles  int32
	totalBytes   int64
	copiedBytes  int64
	lastActivity time.Time
	cancel       context.CancelFunc
}

var (
	requiredSensorCodes = []string{
		"00-00", "00-01", "00-02", "00-03",
		"01-00", "01-01",
		"02-00", "02-01",
		"03-00",
		"04-00", "05-00", "06-00", "07-00",
	}

	// RAW capture file name format (from WU01-WU13 nodes):
	// Lvl0X or Lvl00 - file type (0X=unverified, 00=verified)
	// 00001 - capture number
	// T (optional) - test capture marker
	// Arh2k_mezen_200725 - project name
	// 06-00 - sensor code (00-00, 00-01, 00-02, 01-00, etc.)
	// BD11EBB0_BE00_4BE7_BC66_9DED8D740C2E - unique session ID
	// .raw - file extension
	captureRegex = regexp.MustCompile(`^(Lvl\d+X?)-(\d+)(?:-(T))?-(.+)-(\d+-\d+)-([A-F0-9_]+)\.raw$`)

	// XML metadata file name format (from CU node):
	// EAD - prefix for metadata
	// 00001 - capture number
	// T (optional) - test capture marker
	// Arh2k_mezen_200725 - project name
	// BD11EBB0_BE00_4BE7_BC66_9DED8D740C2E - unique session ID
	// .xml - file extension
	// Note: XML file may be missing for test captures
	metadataRegex = regexp.MustCompile(`^EAD-(\d+)(?:-(T))?-(.+)-([A-F0-9_]+)\.xml$`)

	// RawQv quality file (optional supplemental file per capture)
	rawQvRegex = regexp.MustCompile(`^RawQv-(\d+)(?:-(T))?-(.+)-([A-F0-9_]+)\.dat$`)
)

const defaultDataMountPoint = "/ucdata"

// New creates a new sync service
func New(nodes, shares []string, baseMountDir string) *Service {
	if baseMountDir == "" {
		baseMountDir = "/ucmount"
	}

	requiredSensors := make(map[string]struct{}, len(requiredSensorCodes))
	for _, sensorCode := range requiredSensorCodes {
		requiredSensors[sensorCode] = struct{}{}
	}

	return &Service{
		nodes:           nodes,
		shares:          shares,
		baseMountDir:    baseMountDir,
		requiredSensors: requiredSensors,
		activeTasks:     make(map[string]*taskInfo),
		captureTracker:  make(map[string]map[string]bool),
	}
}

// SetStateStore enables persistent SQLite-backed state for the service.
func (s *Service) SetStateStore(store *state.Store) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.stateStore = store
	if store == nil {
		return nil
	}

	status, err := store.LoadStatus()
	if err != nil {
		return err
	}

	s.project = status.Project
	s.destination = status.Destination
	s.maxParallelism = status.MaxParallelism
	s.isRunning = false
	atomic.StoreInt32(&s.completedCaptures, int32(status.CompletedCaptures))
	atomic.StoreInt32(&s.completedTestCaptures, int32(status.CompletedTestCaptures))
	s.lastCaptureNumber = status.LastCaptureNumber
	s.lastTestCaptureNumber = status.LastTestCaptureNumber
	s.captureTracker = make(map[string]map[string]bool)

	if status.IsRunning {
		return store.StopRun(state.StatusSnapshot{
			Project:               status.Project,
			Destination:           status.Destination,
			MaxParallelism:        status.MaxParallelism,
			CompletedCaptures:     status.CompletedCaptures,
			CompletedTestCaptures: status.CompletedTestCaptures,
			LastCaptureNumber:     status.LastCaptureNumber,
			LastTestCaptureNumber: status.LastTestCaptureNumber,
		})
	}

	return nil
}

// Start begins synchronization
func (s *Service) Start(ctx context.Context, project, destination string, maxParallelism int, forceFullResync bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isRunning {
		return fmt.Errorf("synchronization already running")
	}

	s.project = project
	s.destination = destination
	s.maxParallelism = maxParallelism
	s.forceFullResync = forceFullResync
	s.globalSemaphore = make(chan struct{}, maxParallelism) // Global limit across all tasks
	s.isRunning = true
	s.captureTracker = make(map[string]map[string]bool)
	atomic.StoreInt32(&s.completedCaptures, 0)
	atomic.StoreInt32(&s.completedTestCaptures, 0)
	s.lastCaptureNumber = ""
	s.lastTestCaptureNumber = ""

	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	if err := ensureDestinationReady(destination); err != nil {
		s.isRunning = false
		s.cancel = nil
		return err
	}

	// Create destination directory
	destDir := filepath.Join(destination, project)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		s.isRunning = false
		return fmt.Errorf("failed to create destination: %w", err)
	}

	log.Info().
		Str("project", project).
		Str("destination", destDir).
		Int("parallelism", maxParallelism).
		Msg("Starting synchronization")

	if s.stateStore != nil {
		if forceFullResync {
			if err := s.stateStore.ResetCopiedFiles(project); err != nil {
				s.isRunning = false
				s.cancel = nil
				return fmt.Errorf("failed to reset copied file state: %w", err)
			}
		}

		persisted, err := s.stateStore.StartRun(project, destination, maxParallelism)
		if err != nil {
			s.isRunning = false
			s.cancel = nil
			return fmt.Errorf("failed to initialize persistent state: %w", err)
		}
		atomic.StoreInt32(&s.completedCaptures, int32(persisted.CompletedCaptures))
		atomic.StoreInt32(&s.completedTestCaptures, int32(persisted.CompletedTestCaptures))
		s.lastCaptureNumber = persisted.LastCaptureNumber
		s.lastTestCaptureNumber = persisted.LastTestCaptureNumber
	}

	// Start main sync loop
	s.wg.Add(1)
	go s.syncLoop(ctx, destDir)

	return nil
}

// Stop halts synchronization
func (s *Service) Stop() {
	s.mu.Lock()
	if !s.isRunning {
		s.mu.Unlock()
		return
	}

	cancel := s.cancel
	s.mu.Unlock()

	log.Info().Msg("Stopping synchronization")

	if cancel != nil {
		cancel()
	}

	s.wg.Wait()

	s.mu.Lock()
	statusSnapshot := state.StatusSnapshot{
		Project:               s.project,
		Destination:           s.destination,
		MaxParallelism:        s.maxParallelism,
		CompletedCaptures:     int(atomic.LoadInt32(&s.completedCaptures)),
		CompletedTestCaptures: int(atomic.LoadInt32(&s.completedTestCaptures)),
		LastCaptureNumber:     s.lastCaptureNumber,
		LastTestCaptureNumber: s.lastTestCaptureNumber,
	}
	s.isRunning = false
	s.cancel = nil
	s.activeTasks = make(map[string]*taskInfo)
	s.forceFullResync = false
	s.globalSemaphore = nil // Release semaphore
	store := s.stateStore
	s.mu.Unlock()

	if store != nil {
		if err := store.StopRun(statusSnapshot); err != nil {
			log.Error().Err(err).Msg("Failed to persist stopped synchronization state")
		}
	}

	log.Info().Msg("Synchronization stopped")
}

// GetStatus returns current sync status
func (s *Service) GetStatus() models.SyncStatus {
	s.mu.RLock()
	tasks := make([]models.SyncTask, 0, len(s.activeTasks))
	for _, task := range s.activeTasks {
		progress := 0.0
		if task.totalBytes > 0 {
			progress = float64(atomic.LoadInt64(&task.copiedBytes)) / float64(task.totalBytes) * 100.0
		}

		tasks = append(tasks, models.SyncTask{
			Node:         task.node,
			Share:        task.share,
			Status:       "running",
			LastActivity: task.lastActivity,
			TotalFiles:   int(atomic.LoadInt32(&task.totalFiles)),
			CopiedFiles:  int(atomic.LoadInt32(&task.copiedFiles)),
			FailedFiles:  int(atomic.LoadInt32(&task.failedFiles)),
			TotalBytes:   atomic.LoadInt64(&task.totalBytes),
			CopiedBytes:  atomic.LoadInt64(&task.copiedBytes),
			Progress:     progress,
		})
	}

	// Calculate active file operations (semaphore usage)
	activeOps := 0
	if s.globalSemaphore != nil {
		activeOps = len(s.globalSemaphore)
	}

	status := models.SyncStatus{
		IsRunning:             s.isRunning,
		Project:               s.project,
		Destination:           s.destination,
		MaxParallelism:        s.maxParallelism,
		ActiveFileOperations:  activeOps,
		CompletedCaptures:     int(atomic.LoadInt32(&s.completedCaptures)),
		CompletedTestCaptures: int(atomic.LoadInt32(&s.completedTestCaptures)),
		LastCaptureNumber:     s.lastCaptureNumber,
		LastTestCaptureNumber: s.lastTestCaptureNumber,
		ActiveTasks:           tasks,
	}
	store := s.stateStore
	s.mu.RUnlock()

	if store != nil {
		persisted, err := store.LoadStatus()
		if err == nil {
			status.IsRunning = persisted.IsRunning
			status.Project = persisted.Project
			status.Destination = persisted.Destination
			status.MaxParallelism = persisted.MaxParallelism
			status.CompletedCaptures = persisted.CompletedCaptures
			status.CompletedTestCaptures = persisted.CompletedTestCaptures
			status.LastCaptureNumber = persisted.LastCaptureNumber
			status.LastTestCaptureNumber = persisted.LastTestCaptureNumber
		}
	}

	return status
}

// FindProjects scans network for available projects
func (s *Service) FindProjects(ctx context.Context) ([]models.ProjectInfo, error) {
	projectMap := make(map[string]string) // name -> source
	var mu sync.Mutex

	var wg sync.WaitGroup
	for _, node := range s.nodes {
		for _, share := range s.shares {
			wg.Add(1)
			go func(node, share string) {
				defer wg.Done()

				// Get mount point for this node/share
				shareName := strings.TrimSuffix(share, "$")
				root := filepath.Join(s.baseMountDir, node, shareName)

				entries, err := os.ReadDir(root)
				if err != nil {
					log.Debug().
						Str("node", node).
						Str("share", share).
						Str("path", root).
						Err(err).
						Msg("Cannot read share")
					return
				}

				for _, entry := range entries {
					if !entry.IsDir() {
						continue
					}

					name := entry.Name()
					if !isValidProjectName(name) {
						continue
					}

					mu.Lock()
					if _, exists := projectMap[name]; !exists {
						projectMap[name] = fmt.Sprintf("%s/%s", node, share)
					}
					mu.Unlock()
				}
			}(node, share)
		}
	}

	wg.Wait()

	// Convert to slice and sort
	projects := make([]models.ProjectInfo, 0, len(projectMap))
	for name, source := range projectMap {
		projects = append(projects, models.ProjectInfo{
			Name:   name,
			Source: source,
		})
	}

	if len(projects) == 0 && s.stateStore != nil {
		cachedProjects, err := s.stateStore.LoadProjects()
		if err == nil && len(cachedProjects) > 0 {
			return cachedProjects, nil
		}
	}

	if s.stateStore != nil && len(projects) > 0 {
		if err := s.stateStore.SaveProjects(projects); err != nil {
			log.Error().Err(err).Msg("Failed to persist discovered projects")
		}
	}

	return projects, nil
}

func (s *Service) syncLoop(ctx context.Context, destDir string) {
	defer s.wg.Done()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.syncIteration(ctx, destDir)
		}
	}
}

func (s *Service) syncIteration(ctx context.Context, destDir string) {
	if err := ensureDestinationReady(destDir); err != nil {
		log.Error().Err(err).Str("destination", destDir).Msg("Destination unavailable, skipping sync iteration")
		return
	}

	for _, node := range s.nodes {
		for _, share := range s.shares {
			select {
			case <-ctx.Done():
				return
			default:
			}

			key := fmt.Sprintf("%s-%s", node, share)

			// Get mount point for this node/share
			shareName := strings.TrimSuffix(share, "$")
			mountPoint := filepath.Join(s.baseMountDir, node, shareName)
			source := filepath.Join(mountPoint, s.project)

			// Check if source exists
			if _, err := os.Stat(source); os.IsNotExist(err) {
				continue
			}

			// Check if task already running
			s.mu.RLock()
			_, exists := s.activeTasks[key]
			s.mu.RUnlock()

			if exists {
				continue
			}

			// Check free disk space
			if !s.checkDiskSpace(destDir) {
				continue
			}

			// Start new sync task
			s.startSyncTask(ctx, node, share, source, destDir)
		}
	}
}

func (s *Service) startSyncTask(parentCtx context.Context, node, share, source, dest string) {
	key := fmt.Sprintf("%s-%s", node, share)

	ctx, cancel := context.WithCancel(parentCtx)
	task := &taskInfo{
		node:         node,
		share:        share,
		lastActivity: time.Now(),
		cancel:       cancel,
	}

	s.mu.Lock()
	s.activeTasks[key] = task
	s.mu.Unlock()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer func() {
			s.mu.Lock()
			delete(s.activeTasks, key)
			s.mu.Unlock()
		}()

		if err := s.syncDirectory(ctx, task, source, dest); err != nil {
			if ctx.Err() == nil {
				log.Error().
					Err(err).
					Str("node", node).
					Str("share", share).
					Msg("Sync error")
			}
		}
	}()
}

func (s *Service) syncDirectory(ctx context.Context, task *taskInfo, source, dest string) error {
	// Scan source directory
	files, err := s.scanDirectory(ctx, source, source)
	if err != nil {
		return err
	}

	// Filter files that need copying
	filesToCopy := make([]string, 0)
	var totalBytes int64

	for _, file := range files {
		if s.shouldCopyFile(file, source, dest) {
			filesToCopy = append(filesToCopy, file)
			if info, err := os.Stat(file); err == nil {
				totalBytes += info.Size()
			}
		}
	}

	atomic.StoreInt32(&task.totalFiles, int32(len(filesToCopy)))
	atomic.StoreInt64(&task.totalBytes, totalBytes)

	// Copy files with parallelism (using global semaphore shared across all tasks)
	var wg sync.WaitGroup

	for _, file := range filesToCopy {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case s.globalSemaphore <- struct{}{}:
		}

		wg.Add(1)
		go func(filePath string) {
			defer wg.Done()
			defer func() { <-s.globalSemaphore }()

			if err := s.copyFile(ctx, task, filePath, source, dest); err != nil {
				atomic.AddInt32(&task.failedFiles, 1)
				log.Error().
					Err(err).
					Str("file", filePath).
					Msg("Failed to copy file")
			}
		}(file)
	}

	wg.Wait()
	return nil
}

func (s *Service) scanDirectory(ctx context.Context, root, current string) ([]string, error) {
	var files []string

	entries, err := os.ReadDir(current)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		select {
		case <-ctx.Done():
			return files, ctx.Err()
		default:
		}

		path := filepath.Join(current, entry.Name())

		if entry.IsDir() {
			if isExcludedDirectory(entry.Name()) {
				continue
			}
			subFiles, err := s.scanDirectory(ctx, root, path)
			if err == nil {
				files = append(files, subFiles...)
			}
		} else {
			files = append(files, path)
		}
	}

	return files, nil
}

func (s *Service) shouldCopyFile(sourcePath, sourceRoot, destRoot string) bool {
	relPath, err := filepath.Rel(sourceRoot, sourcePath)
	if err != nil {
		return true
	}
	relPath = filepath.ToSlash(relPath)

	sourceInfo, err := os.Stat(sourcePath)
	if err != nil {
		return true
	}

	s.mu.RLock()
	store := s.stateStore
	project := s.project
	forceFullResync := s.forceFullResync
	s.mu.RUnlock()

	if store != nil && !forceFullResync {
		copied, err := store.IsFileCopied(project, relPath, sourceInfo.Size(), sourceInfo.ModTime())
		if err == nil && copied {
			return false
		}
		if err != nil {
			log.Warn().Err(err).Str("file", relPath).Msg("Failed to check persisted copied file state")
		}
	}

	destPath := filepath.Join(destRoot, relPath)
	destInfo, err := os.Stat(destPath)
	if os.IsNotExist(err) {
		return true
	}
	if err != nil {
		return true
	}

	// Copy if size differs or source is newer (with 2-second tolerance)
	if destInfo.Size() != sourceInfo.Size() {
		return true
	}

	if destInfo.ModTime().Before(sourceInfo.ModTime().Add(-2 * time.Second)) {
		return true
	}

	return false
}

func (s *Service) copyFile(ctx context.Context, task *taskInfo, sourcePath, sourceRoot, destRoot string) error {
	if err := ensureDestinationReady(destRoot); err != nil {
		return err
	}

	relPath, err := filepath.Rel(sourceRoot, sourcePath)
	if err != nil {
		return err
	}

	destPath := filepath.Join(destRoot, relPath)

	// Create destination directory
	destDir := filepath.Dir(destPath)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	// Open source file
	src, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer src.Close()

	// Create destination file
	dst, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer dst.Close()

	// Copy with context cancellation
	written, err := io.Copy(dst, src)
	if err != nil {
		return err
	}

	// Preserve timestamps
	if info, err := src.Stat(); err == nil {
		os.Chtimes(destPath, info.ModTime(), info.ModTime())
	}

	// Update stats
	atomic.AddInt32(&task.copiedFiles, 1)
	atomic.AddInt64(&task.copiedBytes, written)
	task.lastActivity = time.Now()

	s.mu.RLock()
	project := s.project
	store := s.stateStore
	s.mu.RUnlock()
	if store != nil {
		if info, err := src.Stat(); err == nil {
			if err := store.MarkFileCopied(project, relPath, info.Size(), info.ModTime()); err != nil {
				log.Error().Err(err).Str("file", relPath).Msg("Failed to persist copied file state")
			}
		}
	}

	// Track capture completion
	s.trackCaptureCompletion(filepath.Base(sourcePath), task.node)

	return nil
}

func (s *Service) trackCaptureCompletion(filename, node string) {
	if len(s.requiredSensors) == 0 {
		return
	}

	// Try to parse as RAW file first
	info := parseCaptureFileName(filename)
	if info == nil {
		// Try to parse as XML metadata file
		info = parseMetadataFileName(filename)
		if info == nil {
			info = parseRawQvFileName(filename)
			if info == nil {
				return
			}
		}
	}

	if info.CaptureNumber == "" {
		return
	}

	s.mu.RLock()
	project := s.project
	store := s.stateStore
	s.mu.RUnlock()

	if store != nil {
		fileExt := strings.ToLower(filepath.Ext(filename))
		var fileKey string

		switch fileExt {
		case ".raw":
			sensorCode := strings.TrimSpace(info.SensorCode)
			if _, ok := s.requiredSensors[sensorCode]; !ok {
				return
			}
			fileKey = fmt.Sprintf("raw:%s", sensorCode)
		case ".xml":
			fileKey = "xml:CU"
		case ".dat":
			fileKey = "dat:CU"
		default:
			return
		}

		status, completed, err := store.RecordCapture(state.CaptureObservation{
			Project:          project,
			Info:             *info,
			FileKey:          fileKey,
			RequiredRawFiles: len(s.requiredSensors),
			RequireXML:       true,
			RequireDAT:       true,
		})
		if err != nil {
			log.Error().Err(err).Str("capture", info.CaptureNumber).Msg("Failed to persist capture state")
			return
		}

		atomic.StoreInt32(&s.completedCaptures, int32(status.CompletedCaptures))
		atomic.StoreInt32(&s.completedTestCaptures, int32(status.CompletedTestCaptures))
		s.mu.Lock()
		s.lastCaptureNumber = status.LastCaptureNumber
		s.lastTestCaptureNumber = status.LastTestCaptureNumber
		s.mu.Unlock()

		if completed {
			summary := formatCaptureSummary(status.RawCount, status.HasXML, status.HasDAT)
			if info.IsTest {
				log.Info().
					Str("capture", info.CaptureNumber).
					Str("project", info.ProjectName).
					Str("type", info.DataType).
					Bool("verified", info.IsVerified).
					Str("sensor", info.SensorCode).
					Str("session", info.SessionID).
					Int("test_count", status.CompletedTestCaptures).
					Msgf("✓ TEST capture completed (%s)", summary)
			} else {
				log.Info().
					Str("capture", info.CaptureNumber).
					Str("project", info.ProjectName).
					Str("type", info.DataType).
					Bool("verified", info.IsVerified).
					Str("sensor", info.SensorCode).
					Str("session", info.SessionID).
					Int("total_count", status.CompletedCaptures).
					Msgf("✓ Capture completed (%s)", summary)
			}
		}

		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	fileMap, exists := s.captureTracker[info.CaptureNumber]
	if !exists {
		fileMap = make(map[string]bool)
		s.captureTracker[info.CaptureNumber] = fileMap
	}

	// Determine file type based on extension and content
	fileExt := strings.ToLower(filepath.Ext(filename))
	var fileKey string

	if fileExt == ".raw" {
		sensorCode := strings.TrimSpace(info.SensorCode)
		if _, ok := s.requiredSensors[sensorCode]; !ok {
			return
		}
		fileKey = fmt.Sprintf("raw:%s", sensorCode)
	} else if fileExt == ".xml" {
		fileKey = "xml:CU"
	} else if fileExt == ".dat" {
		// RawQv quality data file - optional supplemental file per capture
		fileKey = "dat:CU"
	} else {
		return // Unknown file type
	}

	// Skip if already tracked
	if fileMap[fileKey] {
		return
	}

	fileMap[fileKey] = true

	// Count RAW and XML files separately
	rawCount := 0
	hasXML := false
	hasDAT := false
	for key := range fileMap {
		if strings.HasPrefix(key, "raw:") {
			rawCount++
		} else if key == "xml:CU" {
			hasXML = true
		} else if key == "dat:CU" {
			hasDAT = true
		}
	}

	requiredRAWFiles := len(s.requiredSensors)

	log.Debug().
		Str("capture", info.CaptureNumber).
		Str("node", normalizeNodeName(node)).
		Str("file_type", fileExt).
		Bool("is_test", info.IsTest).
		Int("raw_files", rawCount).
		Bool("has_xml", hasXML).
		Bool("has_dat", hasDAT).
		Int("required_raw_files", requiredRAWFiles).
		Bool("require_xml", true).
		Bool("require_rawqv", true).
		Msgf("Capture progress: %s", formatCaptureSummary(rawCount, hasXML, hasDAT))

	// Check if capture is complete
	isComplete := rawCount == requiredRAWFiles && hasXML && hasDAT

	if isComplete {
		summary := formatCaptureSummary(rawCount, hasXML, hasDAT)

		if info.IsTest {
			atomic.AddInt32(&s.completedTestCaptures, 1)
			s.lastTestCaptureNumber = info.CaptureNumber
			log.Info().
				Str("capture", info.CaptureNumber).
				Str("project", info.ProjectName).
				Str("type", info.DataType).
				Bool("verified", info.IsVerified).
				Str("sensor", info.SensorCode).
				Str("session", info.SessionID).
				Int("test_count", int(atomic.LoadInt32(&s.completedTestCaptures))).
				Msgf("✓ TEST capture completed (%s)", summary)
		} else {
			atomic.AddInt32(&s.completedCaptures, 1)
			s.lastCaptureNumber = info.CaptureNumber
			log.Info().
				Str("capture", info.CaptureNumber).
				Str("project", info.ProjectName).
				Str("type", info.DataType).
				Bool("verified", info.IsVerified).
				Str("sensor", info.SensorCode).
				Str("session", info.SessionID).
				Int("total_count", int(atomic.LoadInt32(&s.completedCaptures))).
				Msgf("✓ Capture completed (%s)", summary)
		}

		delete(s.captureTracker, info.CaptureNumber)
	}
}

func (s *Service) checkDiskSpace(path string) bool {
	// TODO: Implement disk space check
	return true
}

func parseCaptureFileName(filename string) *models.CaptureInfo {
	matches := captureRegex.FindStringSubmatch(filename)
	if len(matches) != 7 {
		return nil
	}

	dataType := matches[1]
	captureNumber := matches[2]
	testMarker := matches[3]
	projectName := matches[4]
	sensorCode := matches[5]
	sessionID := matches[6]

	// Determine if verified: Lvl00 = verified, Lvl0X = unverified
	isVerified := dataType == "Lvl00"

	return &models.CaptureInfo{
		DataType:      dataType,
		CaptureNumber: captureNumber,
		IsTest:        testMarker != "",
		ProjectName:   projectName,
		SensorCode:    sensorCode,
		SessionID:     sessionID,
		IsVerified:    isVerified,
	}
}

func parseMetadataFileName(filename string) *models.CaptureInfo {
	matches := metadataRegex.FindStringSubmatch(filename)
	if len(matches) != 5 {
		return nil
	}

	captureNumber := matches[1]
	testMarker := matches[2]
	projectName := matches[3]
	sessionID := matches[4]

	return &models.CaptureInfo{
		DataType:      "EAD",
		CaptureNumber: captureNumber,
		IsTest:        testMarker != "",
		ProjectName:   projectName,
		SensorCode:    "",
		SessionID:     sessionID,
		IsVerified:    true,
	}
}

func parseRawQvFileName(filename string) *models.CaptureInfo {
	matches := rawQvRegex.FindStringSubmatch(filename)
	if len(matches) != 5 {
		return nil
	}

	return &models.CaptureInfo{
		DataType:      "RawQv",
		CaptureNumber: matches[1],
		IsTest:        matches[2] != "",
		ProjectName:   matches[3],
		SensorCode:    "",
		SessionID:     matches[4],
		IsVerified:    true,
	}
}

func normalizeNodeName(node string) string {
	return strings.ToUpper(strings.TrimSpace(node))
}

func formatCaptureSummary(rawCount int, hasXML, hasDAT bool) string {
	parts := []string{fmt.Sprintf("%d RAW", rawCount)}
	totalFiles := rawCount

	if hasXML {
		parts = append(parts, "1 XML")
		totalFiles++
	}

	if hasDAT {
		parts = append(parts, "1 RawQv")
		totalFiles++
	}

	return fmt.Sprintf("%s = %d files", strings.Join(parts, " + "), totalFiles)
}

func ensureDestinationReady(destination string) error {
	if !requiresMountedDestination(destination) {
		return nil
	}

	mounted, err := isMountPointMounted(defaultDataMountPoint)
	if err != nil {
		return fmt.Errorf("failed to check destination mount %s: %w", defaultDataMountPoint, err)
	}

	if !mounted {
		return fmt.Errorf("destination %s is unavailable: %s is not mounted", destination, defaultDataMountPoint)
	}

	return nil
}

func requiresMountedDestination(destination string) bool {
	clean := filepath.ToSlash(pathpkg.Clean(destination))
	return clean == defaultDataMountPoint || strings.HasPrefix(clean, defaultDataMountPoint+"/")
}

func isMountPointMounted(mountPoint string) (bool, error) {
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return false, err
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == mountPoint {
			return true, nil
		}
	}

	return false, nil
}

func isValidProjectName(name string) bool {
	excluded := []string{
		"system volume information", "recycler", "recycled", "$recycle.bin",
		"logs", "log", "temp", "tmp", "windows", "program files",
	}

	lower := strings.ToLower(name)
	for _, ex := range excluded {
		if lower == ex || strings.HasPrefix(lower, ex+" ") {
			return false
		}
	}

	if strings.HasPrefix(name, "$") || strings.HasPrefix(name, ".") || len(name) <= 1 {
		return false
	}

	return true
}

func isExcludedDirectory(name string) bool {
	excluded := []string{
		"System Volume Information",
		"RECYCLER",
		"RECYCLED",
		"$RECYCLE.BIN",
		".git",
		".svn",
		"node_modules",
	}

	for _, ex := range excluded {
		if strings.EqualFold(name, ex) {
			return true
		}
	}

	return false
}
