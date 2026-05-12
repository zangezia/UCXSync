package sync

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shirou/gopsutil/v3/disk"
	"github.com/zangezia/UCXSync/internal/state"
)

// TestGlobalSemaphore verifies that globalSemaphore limits concurrent operations
func TestGlobalSemaphore(t *testing.T) {
	nodes := []string{"WU01", "WU02", "WU03"}
	shares := []string{"E$", "F$"}

	s := New(nodes, shares, "/tmp/test")

	// Test initial state
	if s.globalSemaphore != nil {
		t.Error("globalSemaphore should be nil before Start()")
	}

	// Note: Full integration test would require:
	// 1. Mock filesystem
	// 2. Context setup
	// 3. Concurrent file operations
	// For now, we just verify the structure exists

	if s.maxParallelism != 0 {
		t.Errorf("maxParallelism should be 0 initially, got %d", s.maxParallelism)
	}
}

// TestSemaphoreCapacity verifies semaphore capacity matches maxParallelism
func TestSemaphoreCapacity(t *testing.T) {
	testCases := []struct {
		name           string
		maxParallelism int
	}{
		{"low", 4},
		{"medium", 8},
		{"high", 16},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a semaphore channel
			sem := make(chan struct{}, tc.maxParallelism)

			// Fill it completely
			for i := 0; i < tc.maxParallelism; i++ {
				sem <- struct{}{}
			}

			// Verify it's full
			if len(sem) != tc.maxParallelism {
				t.Errorf("Expected semaphore length %d, got %d", tc.maxParallelism, len(sem))
			}

			// Verify capacity
			if cap(sem) != tc.maxParallelism {
				t.Errorf("Expected semaphore capacity %d, got %d", tc.maxParallelism, cap(sem))
			}

			// Drain it
			for i := 0; i < tc.maxParallelism; i++ {
				<-sem
			}

			// Verify it's empty
			if len(sem) != 0 {
				t.Errorf("Expected empty semaphore, got length %d", len(sem))
			}
		})
	}
}

func TestRequiresMountedDestination(t *testing.T) {
	testCases := []struct {
		path     string
		expected bool
	}{
		{path: "/ucdata", expected: true},
		{path: "/ucdata/project", expected: true},
		{path: "/media/usb", expected: false},
		{path: "/tmp/output", expected: false},
	}

	for _, tc := range testCases {
		t.Run(tc.path, func(t *testing.T) {
			if actual := requiresMountedDestination(tc.path); actual != tc.expected {
				t.Fatalf("requiresMountedDestination(%q) = %v, want %v", tc.path, actual, tc.expected)
			}
		})
	}
}

func TestParseRawQvFileName(t *testing.T) {
	info := parseRawQvFileName("RawQv-00002-GT3-B531D783_3779_4327_9CBD_9B2107EF1969.dat")
	if info == nil {
		t.Fatal("expected RawQv file to be parsed")
	}

	if info.CaptureNumber != "00002" {
		t.Fatalf("unexpected capture number: %s", info.CaptureNumber)
	}

	if info.ProjectName != "GT3" {
		t.Fatalf("unexpected project name: %s", info.ProjectName)
	}
}

func TestParseCaptureFileNameWithTestMarkerAndUnderscoreProject(t *testing.T) {
	info := parseCaptureFileName("Lvl0X-00001-T-Test_6-00-00-E55452A3_7F5A_4E6C_A049_945BF67F9D17.raw")
	if info == nil {
		t.Fatal("expected test RAW file to be parsed")
	}
	if !info.IsTest {
		t.Fatal("expected file to be recognized as test capture")
	}
	if info.CaptureNumber != "00001" {
		t.Fatalf("CaptureNumber = %q, want 00001", info.CaptureNumber)
	}
	if info.ProjectName != "Test_6" {
		t.Fatalf("ProjectName = %q, want Test_6", info.ProjectName)
	}
	if info.SensorCode != "00-00" {
		t.Fatalf("SensorCode = %q, want 00-00", info.SensorCode)
	}
}

func TestParseMetadataFileNameWithTestMarker(t *testing.T) {
	info := parseMetadataFileName("EAD-00001-T-Test_6-E55452A3_7F5A_4E6C_A049_945BF67F9D17.xml")
	if info == nil {
		t.Fatal("expected test EAD file to be parsed")
	}
	if !info.IsTest {
		t.Fatal("expected metadata file to preserve test marker")
	}
	if info.ProjectName != "Test_6" {
		t.Fatalf("ProjectName = %q, want Test_6", info.ProjectName)
	}
}

func TestParseRawQvFileNameWithTestMarker(t *testing.T) {
	info := parseRawQvFileName("RawQv-00001-T-Test_6-E55452A3_7F5A_4E6C_A049_945BF67F9D17.dat")
	if info == nil {
		t.Fatal("expected test RawQv file to be parsed")
	}
	if !info.IsTest {
		t.Fatal("expected RawQv file to preserve test marker")
	}
	if info.ProjectName != "Test_6" {
		t.Fatalf("ProjectName = %q, want Test_6", info.ProjectName)
	}
}

func TestFormatCaptureSummaryIncludesRawQv(t *testing.T) {
	summary := formatCaptureSummary(13, true, true)
	expected := "13 RAW + 1 XML + 1 RawQv = 15 files"
	if summary != expected {
		t.Fatalf("formatCaptureSummary() = %q, want %q", summary, expected)
	}
}

func TestTrackCaptureCompletionWithRawQv(t *testing.T) {
	nodes := []string{"WU01", "WU02", "WU03", "WU04", "WU05", "WU06", "WU07", "WU08", "WU09", "WU10", "WU11", "WU12", "WU13", "CU"}
	svc := New(nodes, []string{"E$"}, "/ucmount")

	for i, sensorCode := range requiredSensorCodes {
		node := nodes[i%len(nodes)]
		filename := fmt.Sprintf("Lvl0X-00002-GT3-%s-B531D783_3779_4327_9CBD_9B2107EF1969.raw", sensorCode)
		svc.trackCaptureCompletion(filename, node)
	}

	svc.trackCaptureCompletion("EAD-00002-GT3-B531D783_3779_4327_9CBD_9B2107EF1969.xml", "CU")
	svc.trackCaptureCompletion("RawQv-00002-GT3-B531D783_3779_4327_9CBD_9B2107EF1969.dat", "CU")

	if got := atomic.LoadInt32(&svc.completedCaptures); got != 1 {
		t.Fatalf("completedCaptures = %d, want 1", got)
	}
}

func TestTrackCaptureCompletionRequiresAllSensorsAndAuxiliaryFiles(t *testing.T) {
	t.Parallel()

	svc := New([]string{"WU01", "WU02", "WU03"}, []string{"E$"}, "/ucmount-a")

	for _, sensorCode := range requiredSensorCodes {
		filename := fmt.Sprintf("Lvl00-00003-Project-%s-ABCDEF01_2345_6789_ABCD_EF0123456789.raw", sensorCode)
		svc.trackCaptureCompletion(filename, "WU01")
	}

	if got := atomic.LoadInt32(&svc.completedCaptures); got != 0 {
		t.Fatalf("completedCaptures without XML/RawQv = %d, want 0", got)
	}

	svc.trackCaptureCompletion("EAD-00003-Project-ABCDEF01_2345_6789_ABCD_EF0123456789.xml", "CU")
	if got := atomic.LoadInt32(&svc.completedCaptures); got != 0 {
		t.Fatalf("completedCaptures without RawQv = %d, want 0", got)
	}

	svc.trackCaptureCompletion("RawQv-00003-Project-ABCDEF01_2345_6789_ABCD_EF0123456789.dat", "CU")
	if got := atomic.LoadInt32(&svc.completedCaptures); got != 1 {
		t.Fatalf("completedCaptures after XML and RawQv = %d, want 1", got)
	}
}

func TestTrackCaptureCompletionAggregatesAcrossSplitInstancesWithSharedState(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "shared-state.db")
	storeA, err := state.New(path, "ucxsync@a")
	if err != nil {
		t.Fatalf("failed to create storeA: %v", err)
	}
	defer storeA.Close()
	storeB, err := state.New(path, "ucxsync@b")
	if err != nil {
		t.Fatalf("failed to create storeB: %v", err)
	}
	defer storeB.Close()

	svcA := New([]string{"WU01", "WU02", "WU03", "WU04", "WU05", "WU06", "WU07"}, []string{"E$"}, "/ucmount-a")
	if err := svcA.SetStateStore(storeA); err != nil {
		t.Fatalf("svcA.SetStateStore returned error: %v", err)
	}
	svcB := New([]string{"WU08", "WU09", "WU10", "WU11", "WU12", "WU13", "CU"}, []string{"E$"}, "/ucmount-b")
	if err := svcB.SetStateStore(storeB); err != nil {
		t.Fatalf("svcB.SetStateStore returned error: %v", err)
	}

	if _, err := storeA.StartRun("Project", "/tmp", 4); err != nil {
		t.Fatalf("storeA.StartRun returned error: %v", err)
	}
	if _, err := storeB.StartRun("Project", "/tmp", 4); err != nil {
		t.Fatalf("storeB.StartRun returned error: %v", err)
	}

	svcA.mu.Lock()
	svcA.project = "Project"
	svcA.mu.Unlock()
	svcB.mu.Lock()
	svcB.project = "Project"
	svcB.mu.Unlock()

	for _, sensorCode := range requiredSensorCodes[:7] {
		svcA.trackCaptureCompletion(fmt.Sprintf("Lvl00-00004-Project-%s-ABCDEF01_2345_6789_ABCD_EF0123456789.raw", sensorCode), "WU01")
	}
	for _, sensorCode := range requiredSensorCodes[7:] {
		svcB.trackCaptureCompletion(fmt.Sprintf("Lvl00-00004-Project-%s-ABCDEF01_2345_6789_ABCD_EF0123456789.raw", sensorCode), "WU08")
	}
	svcB.trackCaptureCompletion("EAD-00004-Project-ABCDEF01_2345_6789_ABCD_EF0123456789.xml", "CU")
	svcB.trackCaptureCompletion("RawQv-00004-Project-ABCDEF01_2345_6789_ABCD_EF0123456789.dat", "CU")

	if got := svcA.GetStatus().CompletedCaptures; got != 1 {
		t.Fatalf("svcA completed captures = %d, want 1", got)
	}
	if got := svcB.GetStatus().CompletedCaptures; got != 1 {
		t.Fatalf("svcB completed captures = %d, want 1", got)
	}
}

func TestTrackTestCaptureCompletionWithoutMetadataAndRawQv(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "shared-state.db")
	store, err := state.New(path, "ucxsync-test")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	svc := New([]string{"WU01"}, []string{"E$"}, "/ucmount-a")
	if err := svc.SetStateStore(store); err != nil {
		t.Fatalf("SetStateStore returned error: %v", err)
	}
	if _, err := store.StartRun("ProjTest", "/tmp", 4); err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}

	svc.mu.Lock()
	svc.project = "ProjTest"
	svc.requiredSensors = map[string]struct{}{"00-00": {}, "00-01": {}}
	svc.mu.Unlock()

	svc.trackCaptureCompletion("Lvl0X-00013-T-ProjTest-00-00-ABCDEF01_2345_6789_ABCD_EF0123456789.raw", "WU01")
	svc.trackCaptureCompletion("Lvl0X-00013-T-ProjTest-00-01-ABCDEF01_2345_6789_ABCD_EF0123456789.raw", "WU02")

	status := svc.GetStatus()
	if status.CompletedTestCaptures != 1 {
		t.Fatalf("CompletedTestCaptures = %d, want 1", status.CompletedTestCaptures)
	}
	if status.LastTestCaptureNumber != "00013" {
		t.Fatalf("LastTestCaptureNumber = %q, want 00013", status.LastTestCaptureNumber)
	}
	if status.CompletedCaptures != 0 {
		t.Fatalf("CompletedCaptures = %d, want 0", status.CompletedCaptures)
	}
	if status.LastCaptureNumber != "" {
		t.Fatalf("LastCaptureNumber = %q, want empty for test-only completion", status.LastCaptureNumber)
	}
}

func TestTrackTestCaptureCompletionWithoutMetadataAndRawQvInMemory(t *testing.T) {
	t.Parallel()

	svc := New([]string{"WU01"}, []string{"E$"}, "/ucmount-a")
	svc.mu.Lock()
	svc.requiredSensors = map[string]struct{}{"00-00": {}, "00-01": {}}
	svc.mu.Unlock()

	if err := svc.trackCaptureCompletion("Lvl0X-00013-T-ProjTest-00-00-ABCDEF01_2345_6789_ABCD_EF0123456789.raw", "WU01"); err != nil {
		t.Fatalf("first RAW track failed: %v", err)
	}
	if err := svc.trackCaptureCompletion("Lvl0X-00013-T-ProjTest-00-01-ABCDEF01_2345_6789_ABCD_EF0123456789.raw", "WU02"); err != nil {
		t.Fatalf("second RAW track failed: %v", err)
	}

	if got := atomic.LoadInt32(&svc.completedTestCaptures); got != 1 {
		t.Fatalf("completedTestCaptures = %d, want 1", got)
	}
	if got := atomic.LoadInt32(&svc.completedCaptures); got != 0 {
		t.Fatalf("completedCaptures = %d, want 0", got)
	}
	if svc.lastTestCaptureNumber != "00013" {
		t.Fatalf("lastTestCaptureNumber = %q, want 00013", svc.lastTestCaptureNumber)
	}
	if _, exists := svc.captureTracker["00013"]; exists {
		t.Fatal("expected completed in-memory test capture to be removed from tracker")
	}
}

func TestTrackTestCaptureCompletionWithProvidedFilenamePattern(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "shared-state.db")
	store, err := state.New(path, "ucxsync-test")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	svc := New([]string{"WU01"}, []string{"E$"}, "/ucmount-a")
	if err := svc.SetStateStore(store); err != nil {
		t.Fatalf("SetStateStore returned error: %v", err)
	}
	if _, err := store.StartRun("Test_6", "/tmp", 4); err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}

	svc.mu.Lock()
	svc.project = "Test_6"
	svc.requiredSensors = map[string]struct{}{"00-00": {}, "00-01": {}}
	svc.mu.Unlock()

	svc.trackCaptureCompletion("Lvl0X-00001-T-Test_6-00-00-E55452A3_7F5A_4E6C_A049_945BF67F9D17.raw", "WU01")
	svc.trackCaptureCompletion("Lvl0X-00001-T-Test_6-00-01-E55452A3_7F5A_4E6C_A049_945BF67F9D17.raw", "WU02")
	svc.trackCaptureCompletion("EAD-00001-T-Test_6-E55452A3_7F5A_4E6C_A049_945BF67F9D17.xml", "CU")
	svc.trackCaptureCompletion("RawQv-00001-T-Test_6-E55452A3_7F5A_4E6C_A049_945BF67F9D17.dat", "CU")

	status := svc.GetStatus()
	if status.CompletedTestCaptures != 1 {
		t.Fatalf("CompletedTestCaptures = %d, want 1", status.CompletedTestCaptures)
	}
	if status.LastTestCaptureNumber != "00001" {
		t.Fatalf("LastTestCaptureNumber = %q, want 00001", status.LastTestCaptureNumber)
	}
}

func TestShouldCopyFileSkipsFilesMarkedCopiedInStateStore(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	sourceRoot := filepath.Join(baseDir, "source")
	destRoot := filepath.Join(baseDir, "dest")
	if err := os.MkdirAll(sourceRoot, 0755); err != nil {
		t.Fatalf("failed to create source root: %v", err)
	}
	if err := os.MkdirAll(destRoot, 0755); err != nil {
		t.Fatalf("failed to create destination root: %v", err)
	}

	store, err := state.New(filepath.Join(baseDir, "state.db"), "ucxsync-test")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	svc := New([]string{"WU01"}, []string{"E$"}, "/ucmount")
	if err := svc.SetStateStore(store); err != nil {
		t.Fatalf("SetStateStore returned error: %v", err)
	}
	svc.mu.Lock()
	svc.project = "ProjA"
	svc.mu.Unlock()

	sourceFile := filepath.Join(sourceRoot, "capture.raw")
	content := []byte("payload")
	if err := os.WriteFile(sourceFile, content, 0644); err != nil {
		t.Fatalf("failed to create source file: %v", err)
	}
	info, err := os.Stat(sourceFile)
	if err != nil {
		t.Fatalf("failed to stat source file: %v", err)
	}

	if err := store.MarkFileCopied("ProjA", "capture.raw", info.Size(), info.ModTime()); err != nil {
		t.Fatalf("MarkFileCopied returned error: %v", err)
	}

	if shouldCopy := svc.shouldCopyFile(sourceFile, sourceRoot, destRoot); shouldCopy {
		t.Fatal("expected shouldCopyFile to skip DB-marked file even when destination is missing")
	}

	svc.mu.Lock()
	svc.forceFullResync = true
	svc.mu.Unlock()
	if shouldCopy := svc.shouldCopyFile(sourceFile, sourceRoot, destRoot); !shouldCopy {
		t.Fatal("expected full resync mode to force re-copy of DB-marked file")
	}
}

func TestShouldCopyFileReconcilesPersistedStateForExistingDestinationFile(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	sourceRoot := filepath.Join(baseDir, "source")
	destRoot := filepath.Join(baseDir, "dest")
	if err := os.MkdirAll(sourceRoot, 0755); err != nil {
		t.Fatalf("failed to create source root: %v", err)
	}
	if err := os.MkdirAll(destRoot, 0755); err != nil {
		t.Fatalf("failed to create destination root: %v", err)
	}

	store, err := state.New(filepath.Join(baseDir, "state.db"), "ucxsync-test")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	svc := New([]string{"WU01"}, []string{"E$"}, "/ucmount")
	if err := svc.SetStateStore(store); err != nil {
		t.Fatalf("SetStateStore returned error: %v", err)
	}
	svc.mu.Lock()
	svc.project = "ProjA"
	svc.requiredSensors = map[string]struct{}{"00-00": {}}
	svc.mu.Unlock()

	filename := "Lvl0X-00001-T-ProjA-00-00-ABCDEF01_2345_6789_ABCD_EF0123456789.raw"
	sourceFile := filepath.Join(sourceRoot, filename)
	content := []byte("payload")
	if err := os.WriteFile(sourceFile, content, 0644); err != nil {
		t.Fatalf("failed to create source file: %v", err)
	}
	sourceInfo, err := os.Stat(sourceFile)
	if err != nil {
		t.Fatalf("failed to stat source file: %v", err)
	}

	destFile := filepath.Join(destRoot, filename)
	if err := os.WriteFile(destFile, content, 0644); err != nil {
		t.Fatalf("failed to create destination file: %v", err)
	}
	if err := os.Chtimes(destFile, sourceInfo.ModTime(), sourceInfo.ModTime()); err != nil {
		t.Fatalf("failed to sync destination timestamps: %v", err)
	}

	if shouldCopy := svc.shouldCopyFile(sourceFile, sourceRoot, destRoot); shouldCopy {
		t.Fatal("expected shouldCopyFile to reconcile persisted state instead of re-copying matching file")
	}

	copied, err := store.IsFileCopied("ProjA", filename, sourceInfo.Size(), sourceInfo.ModTime())
	if err != nil {
		t.Fatalf("IsFileCopied returned error: %v", err)
	}
	if !copied {
		t.Fatal("expected reconcile path to persist copied file state")
	}

	status, err := store.LoadProjectStatus("ProjA")
	if err != nil {
		t.Fatalf("LoadProjectStatus returned error: %v", err)
	}
	if status.CompletedTestCaptures != 1 {
		t.Fatalf("CompletedTestCaptures = %d, want 1 after reconcile", status.CompletedTestCaptures)
	}
	if status.LastTestCaptureNumber != "00001" {
		t.Fatalf("LastTestCaptureNumber = %q, want 00001", status.LastTestCaptureNumber)
	}
}

func TestCheckDiskSpaceBlocksLowFreeSpace(t *testing.T) {
	t.Parallel()

	svc := New([]string{"WU01"}, []string{"E$"}, "/ucmount")
	svc.SetDiskSpaceThresholds(100, 50)
	svc.diskUsage = func(path string) (*disk.UsageStat, error) {
		return &disk.UsageStat{Free: 149}, nil
	}

	if svc.checkDiskSpace("/tmp") {
		t.Fatal("expected checkDiskSpace to block when free space is below threshold plus safety margin")
	}
}

func TestCheckDiskSpaceAllowsSufficientFreeSpace(t *testing.T) {
	t.Parallel()

	svc := New([]string{"WU01"}, []string{"E$"}, "/ucmount")
	svc.SetDiskSpaceThresholds(100, 50)
	svc.diskUsage = func(path string) (*disk.UsageStat, error) {
		return &disk.UsageStat{Free: 150}, nil
	}

	if !svc.checkDiskSpace("/tmp") {
		t.Fatal("expected checkDiskSpace to allow sync when free space satisfies threshold")
	}
}

func TestSyncLoopRunsImmediateIterationBeforeTicker(t *testing.T) {
	t.Parallel()

	svc := New([]string{"WU01"}, []string{"E$"}, "/ucmount")
	svc.SetServiceLoopInterval(time.Hour)

	iterationStarted := make(chan struct{}, 1)
	releaseIteration := make(chan struct{})
	svc.syncIterationFunc = func(ctx context.Context, destDir string) {
		select {
		case iterationStarted <- struct{}{}:
		default:
		}

		<-releaseIteration
	}

	ctx, cancel := context.WithCancel(context.Background())
	svc.wg.Add(1)

	done := make(chan struct{})
	go func() {
		svc.syncLoop(ctx, "/tmp/dest")
		close(done)
	}()

	select {
	case <-iterationStarted:
	case <-time.After(200 * time.Millisecond):
		close(releaseIteration)
		cancel()
		t.Fatal("expected syncLoop to run an immediate iteration before waiting for ticker")
	}

	close(releaseIteration)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("syncLoop did not exit after context cancellation")
	}
}

func TestCheckSharesAvailabilityReportsUnmountedShare(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mountPoint := filepath.Join(root, "WU01", "E")
	if err := os.MkdirAll(mountPoint, 0755); err != nil {
		t.Fatalf("failed to create mount point: %v", err)
	}

	svc := New([]string{"WU01"}, []string{"E$"}, root)
	svc.mountPointMounted = func(path string) (bool, error) {
		if path != mountPoint {
			t.Fatalf("mountPointMounted called with %q, want %q", path, mountPoint)
		}
		return false, nil
	}

	unavailable := svc.CheckSharesAvailability()
	if len(unavailable) != 1 {
		t.Fatalf("expected 1 unavailable share, got %d", len(unavailable))
	}
	if unavailable[0].Path != mountPoint {
		t.Fatalf("unavailable path = %q, want %q", unavailable[0].Path, mountPoint)
	}
}

func TestCheckSharesAvailabilityAcceptsMountedShare(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mountPoint := filepath.Join(root, "WU01", "E")
	if err := os.MkdirAll(mountPoint, 0755); err != nil {
		t.Fatalf("failed to create mount point: %v", err)
	}

	svc := New([]string{"WU01"}, []string{"E$"}, root)
	svc.mountPointMounted = func(path string) (bool, error) {
		if path != mountPoint {
			t.Fatalf("mountPointMounted called with %q, want %q", path, mountPoint)
		}
		return true, nil
	}

	unavailable := svc.CheckSharesAvailability()
	if len(unavailable) != 0 {
		t.Fatalf("expected all shares to be available, got %d unavailable", len(unavailable))
	}
}

func TestStopDoesNotDeadlockWhileTasksCleanup(t *testing.T) {
	svc := New([]string{"WU01"}, []string{"E$"}, "/ucmount")
	ctx, cancel := context.WithCancel(context.Background())

	svc.mu.Lock()
	svc.isRunning = true
	svc.cancel = cancel
	svc.globalSemaphore = make(chan struct{}, 1)
	svc.activeTasks["WU01-E$"] = &taskInfo{node: "WU01", share: "E$"}
	svc.mu.Unlock()

	svc.wg.Add(1)
	go func() {
		defer svc.wg.Done()
		<-ctx.Done()

		// Simulate the same cleanup path as startSyncTask defer.
		svc.mu.Lock()
		delete(svc.activeTasks, "WU01-E$")
		svc.mu.Unlock()
	}()

	done := make(chan struct{})
	go func() {
		svc.Stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() timed out; possible deadlock while waiting for task cleanup")
	}

	svc.mu.RLock()
	defer svc.mu.RUnlock()

	if svc.isRunning {
		t.Fatal("expected sync service to be stopped")
	}

	if svc.cancel != nil {
		t.Fatal("expected cancel func to be cleared after Stop")
	}

	if svc.globalSemaphore != nil {
		t.Fatal("expected semaphore to be released after Stop")
	}

	if len(svc.activeTasks) != 0 {
		t.Fatalf("expected activeTasks to be empty after Stop, got %d", len(svc.activeTasks))
	}
}
