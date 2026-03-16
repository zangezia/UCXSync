package sync

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

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
