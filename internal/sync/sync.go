package sync

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/zangezia/UCXSync/pkg/models"
)

// Service handles file synchronization operations
type Service struct {
	nodes         []string
	shares        []string
	baseMountDir  string // Base directory for mounted shares (e.g., /mnt/ucx)

	mu                    sync.RWMutex
	isRunning             bool
	project               string
	destination           string
	maxParallelism        int
	activeTasks           map[string]*taskInfo
	captureTracker        map[string]map[string]bool // capture# -> node -> completed
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
	captureRegex = regexp.MustCompile(`^(Lvl\d+X)-(\d+)-(T-)?([^-]+)-\d+-\d+-([A-F0-9_]+)\.raw$`)
)

// New creates a new sync service
func New(nodes, shares []string, baseMountDir string) *Service {
	if baseMountDir == "" {
		baseMountDir = "/mnt/ucx"
	}
	return &Service{
		nodes:          nodes,
		shares:         shares,
		baseMountDir:   baseMountDir,
		activeTasks:    make(map[string]*taskInfo),
		captureTracker: make(map[string]map[string]bool),
	}
}

// Start begins synchronization
func (s *Service) Start(ctx context.Context, project, destination string, maxParallelism int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isRunning {
		return fmt.Errorf("synchronization already running")
	}

	s.project = project
	s.destination = destination
	s.maxParallelism = maxParallelism
	s.isRunning = true

	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

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

	// Start main sync loop
	s.wg.Add(1)
	go s.syncLoop(ctx, destDir)

	return nil
}

// Stop halts synchronization
func (s *Service) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.isRunning {
		return
	}

	log.Info().Msg("Stopping synchronization")

	if s.cancel != nil {
		s.cancel()
	}

	s.wg.Wait()

	s.isRunning = false
	s.activeTasks = make(map[string]*taskInfo)

	log.Info().Msg("Synchronization stopped")
}

// GetStatus returns current sync status
func (s *Service) GetStatus() models.SyncStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

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

	return models.SyncStatus{
		IsRunning:             s.isRunning,
		Project:               s.project,
		Destination:           s.destination,
		CompletedCaptures:     int(atomic.LoadInt32(&s.completedCaptures)),
		CompletedTestCaptures: int(atomic.LoadInt32(&s.completedTestCaptures)),
		LastCaptureNumber:     s.lastCaptureNumber,
		LastTestCaptureNumber: s.lastTestCaptureNumber,
		ActiveTasks:           tasks,
	}
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

	// Copy files with parallelism
	sem := make(chan struct{}, s.maxParallelism)
	var wg sync.WaitGroup

	for _, file := range filesToCopy {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case sem <- struct{}{}:
		}

		wg.Add(1)
		go func(filePath string) {
			defer wg.Done()
			defer func() { <-sem }()

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

	destPath := filepath.Join(destRoot, relPath)
	destInfo, err := os.Stat(destPath)
	if os.IsNotExist(err) {
		return true
	}
	if err != nil {
		return true
	}

	sourceInfo, err := os.Stat(sourcePath)
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

	// Track capture completion
	s.trackCaptureCompletion(filepath.Base(sourcePath), task.node)

	return nil
}

func (s *Service) trackCaptureCompletion(filename, node string) {
	info := parseCaptureFileName(filename)
	if info == nil || info.CaptureNumber == "" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	nodeMap, exists := s.captureTracker[info.CaptureNumber]
	if !exists {
		nodeMap = make(map[string]bool)
		s.captureTracker[info.CaptureNumber] = nodeMap
	}

	nodeMap[node] = true

	// Check if all 13 worker nodes completed
	if len(nodeMap) == 13 {
		if info.IsTest {
			atomic.AddInt32(&s.completedTestCaptures, 1)
			s.lastTestCaptureNumber = info.CaptureNumber
			log.Info().
				Str("capture", info.CaptureNumber).
				Str("project", info.ProjectName).
				Int("test_count", int(atomic.LoadInt32(&s.completedTestCaptures))).
				Msg("✓ TEST capture completed (13/13)")
		} else {
			atomic.AddInt32(&s.completedCaptures, 1)
			s.lastCaptureNumber = info.CaptureNumber
			log.Info().
				Str("capture", info.CaptureNumber).
				Str("project", info.ProjectName).
				Int("total_count", int(atomic.LoadInt32(&s.completedCaptures))).
				Msg("✓ Capture completed (13/13)")
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
	if len(matches) != 6 {
		return nil
	}

	return &models.CaptureInfo{
		DataType:      matches[1],
		CaptureNumber: matches[2],
		IsTest:        matches[3] != "",
		ProjectName:   matches[4],
		SessionID:     matches[5],
	}
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
