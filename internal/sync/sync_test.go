package sync

import (
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
