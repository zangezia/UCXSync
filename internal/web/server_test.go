package web

import (
	"testing"

	"github.com/zangezia/UCXSync/pkg/models"
)

func TestBuildDashboardSummaryUsesMinimumCompletedCapturesAcrossInstances(t *testing.T) {
	t.Parallel()

	server := &Server{}
	states := []models.DashboardInstanceState{
		{
			Available: true,
			Status: models.SyncStatus{
				IsRunning:             true,
				CompletedCaptures:     12,
				CompletedTestCaptures: 4,
				ActiveFileOperations:  3,
				MaxParallelism:        4,
				ActiveTasks:           []models.SyncTask{{Node: "WU01"}},
			},
		},
		{
			Available: true,
			Status: models.SyncStatus{
				IsRunning:             true,
				CompletedCaptures:     10,
				CompletedTestCaptures: 2,
				ActiveFileOperations:  5,
				MaxParallelism:        6,
				ActiveTasks:           []models.SyncTask{{Node: "WU08"}, {Node: "WU09"}},
			},
		},
	}

	summary := server.buildDashboardSummary(states)

	if summary.TotalCompletedCaptures != 10 {
		t.Fatalf("TotalCompletedCaptures = %d, want 10", summary.TotalCompletedCaptures)
	}

	if summary.TotalCompletedTest != 2 {
		t.Fatalf("TotalCompletedTest = %d, want 2", summary.TotalCompletedTest)
	}

	if summary.TotalActiveFileOps != 8 {
		t.Fatalf("TotalActiveFileOps = %d, want 8", summary.TotalActiveFileOps)
	}

	if summary.TotalMaxParallelism != 10 {
		t.Fatalf("TotalMaxParallelism = %d, want 10", summary.TotalMaxParallelism)
	}

	if summary.TotalActiveTasks != 3 {
		t.Fatalf("TotalActiveTasks = %d, want 3", summary.TotalActiveTasks)
	}
}
