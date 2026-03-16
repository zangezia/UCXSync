package web

import (
	"path/filepath"
	"testing"

	"github.com/zangezia/UCXSync/internal/state"
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
				LastCaptureNumber:     "00011",
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
				LastCaptureNumber:     "00012",
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

	if summary.LastCaptureNumber != "00012" {
		t.Fatalf("LastCaptureNumber = %q, want 00012", summary.LastCaptureNumber)
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

func TestBuildDashboardSummaryUsesSharedStateStoreForCommonCaptureStats(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "state.db")
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

	if _, err := storeA.StartRun("ProjA", "/ucdata", 4); err != nil {
		t.Fatalf("storeA StartRun returned error: %v", err)
	}
	if _, err := storeB.StartRun("ProjA", "/ucdata", 4); err != nil {
		t.Fatalf("storeB StartRun returned error: %v", err)
	}

	for _, step := range []struct {
		store   *state.Store
		fileKey string
		sensor  string
	}{
		{store: storeA, fileKey: "raw:00-00", sensor: "00-00"},
		{store: storeB, fileKey: "raw:00-01", sensor: "00-01"},
		{store: storeA, fileKey: "xml:CU"},
		{store: storeB, fileKey: "dat:CU"},
	} {
		_, _, err := step.store.RecordCapture(state.CaptureObservation{
			Project: "ProjA",
			Info: models.CaptureInfo{
				DataType:      "Lvl00",
				CaptureNumber: "00077",
				ProjectName:   "ProjA",
				SensorCode:    step.sensor,
				SessionID:     "SESSION_A",
				IsVerified:    true,
			},
			FileKey:          step.fileKey,
			RequiredRawFiles: 2,
			RequireXML:       true,
			RequireDAT:       true,
		})
		if err != nil {
			t.Fatalf("RecordCapture(%s) returned error: %v", step.fileKey, err)
		}
	}

	server := &Server{stateStore: storeA}
	states := []models.DashboardInstanceState{
		{
			Available: true,
			Status: models.SyncStatus{
				Project:               "ProjA",
				CompletedCaptures:     0,
				CompletedTestCaptures: 0,
				LastCaptureNumber:     "",
			},
		},
		{
			Available: true,
			Status: models.SyncStatus{
				Project:               "ProjA",
				CompletedCaptures:     0,
				CompletedTestCaptures: 0,
				LastCaptureNumber:     "",
			},
		},
	}

	summary := server.buildDashboardSummary(states)

	if summary.TotalCompletedCaptures != 1 {
		t.Fatalf("TotalCompletedCaptures = %d, want 1", summary.TotalCompletedCaptures)
	}
	if summary.LastCaptureNumber != "00077" {
		t.Fatalf("LastCaptureNumber = %q, want 00077", summary.LastCaptureNumber)
	}
}

func TestBuildDashboardSummaryUsesLatestTestCaptureAsCommonLastCapture(t *testing.T) {
	t.Parallel()

	server := &Server{}
	states := []models.DashboardInstanceState{
		{
			Available: true,
			Status: models.SyncStatus{
				CompletedCaptures:     12,
				CompletedTestCaptures: 1,
				LastCaptureNumber:     "00012",
				LastTestCaptureNumber: "00013",
			},
		},
		{
			Available: true,
			Status: models.SyncStatus{
				CompletedCaptures:     12,
				CompletedTestCaptures: 1,
				LastCaptureNumber:     "00012",
				LastTestCaptureNumber: "00013",
			},
		},
	}

	summary := server.buildDashboardSummary(states)
	if summary.LastCaptureNumber != "00013" {
		t.Fatalf("LastCaptureNumber = %q, want 00013", summary.LastCaptureNumber)
	}
	if summary.TotalCompletedTest != 1 {
		t.Fatalf("TotalCompletedTest = %d, want 1", summary.TotalCompletedTest)
	}
}
