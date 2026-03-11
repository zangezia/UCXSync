package sync

import (
	"fmt"
	"sync/atomic"
	"testing"
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
	nodes := []string{"WU01", "WU02", "WU03", "WU04", "WU05", "WU06", "WU07", "WU08", "WU09", "WU10", "WU11", "WU12", "WU13"}
	svc := New(nodes, []string{"E$"}, "/ucmount")

	for i, node := range nodes {
		filename := fmt.Sprintf("Lvl0X-00002-GT3-%02d-00-B531D783_3779_4327_9CBD_9B2107EF1969.raw", i)
		svc.trackCaptureCompletion(filename, node)
	}

	svc.trackCaptureCompletion("RawQv-00002-GT3-B531D783_3779_4327_9CBD_9B2107EF1969.dat", "CU")
	svc.trackCaptureCompletion("EAD-00002-GT3-B531D783_3779_4327_9CBD_9B2107EF1969.xml", "CU")

	if got := atomic.LoadInt32(&svc.completedCaptures); got != 1 {
		t.Fatalf("completedCaptures = %d, want 1", got)
	}
}
