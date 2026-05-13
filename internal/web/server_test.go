package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zangezia/UCXSync/internal/config"
	"github.com/zangezia/UCXSync/internal/state"
	syncService "github.com/zangezia/UCXSync/internal/sync"
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

func TestAutoRemountSharesRetriesWhenSharesUnavailable(t *testing.T) {
	t.Parallel()

	var mountCalls atomic.Int32
	mounted := atomic.Bool{}
	server := &Server{
		cfg:                      &config.Config{Sync: config.Sync{ServiceLoopInterval: 10 * time.Millisecond}},
		checkNetworkRequirements: func() error { return nil },
		checkSharesAvailability: func() []syncService.UnavailableShare {
			if mounted.Load() {
				return nil
			}
			return []syncService.UnavailableShare{{Node: "WU01", Share: "E$", Path: "/ucmount/WU01/E"}}
		},
		mountSharesFunc: func() error {
			mountCalls.Add(1)
			mounted.Store(true)
			return nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		server.autoRemountShares(ctx)
		close(done)
	}()

	deadline := time.After(500 * time.Millisecond)
	for mountCalls.Load() == 0 {
		select {
		case <-deadline:
			cancel()
			t.Fatal("expected automatic remount attempt")
		case <-time.After(10 * time.Millisecond):
		}
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("autoRemountShares did not stop after context cancellation")
	}
}

func TestAutoRemountSharesAttemptsImmediatelyWhenSharesUnavailable(t *testing.T) {
	t.Parallel()

	remountStarted := make(chan struct{}, 1)
	server := &Server{
		cfg:                      &config.Config{Sync: config.Sync{ServiceLoopInterval: time.Hour}},
		checkNetworkRequirements: func() error { return nil },
		checkSharesAvailability: func() []syncService.UnavailableShare {
			return []syncService.UnavailableShare{{Node: "WU01", Share: "E$", Path: "/ucmount/WU01/E"}}
		},
		mountSharesFunc: func() error {
			select {
			case remountStarted <- struct{}{}:
			default:
			}
			return nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		server.autoRemountShares(ctx)
		close(done)
	}()

	select {
	case <-remountStarted:
	case <-time.After(100 * time.Millisecond):
		cancel()
		t.Fatal("expected immediate remount attempt before first ticker interval")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("autoRemountShares did not stop after context cancellation")
	}
}

func TestBuildPreflightStatusReady(t *testing.T) {
	t.Parallel()

	server := newPreflightTestServer(models.SyncStatus{}, nil)

	preflight := server.buildPreflightStatus(context.Background(), "ProjA", "/ucdata")

	if !preflight.Ready {
		t.Fatalf("preflight.Ready = false, want true")
	}

	if len(preflight.Checks) != 5 {
		t.Fatalf("len(preflight.Checks) = %d, want 5", len(preflight.Checks))
	}

	for _, check := range preflight.Checks {
		if check.Status != "ready" {
			t.Fatalf("check %q status = %q, want ready", check.Key, check.Status)
		}
	}

	if preflight.AvailableProjects != 1 {
		t.Fatalf("AvailableProjects = %d, want 1", preflight.AvailableProjects)
	}
	if preflight.AvailableDestinations != 1 {
		t.Fatalf("AvailableDestinations = %d, want 1", preflight.AvailableDestinations)
	}
}

func TestBuildPreflightStatusMissingProjectBlocksStart(t *testing.T) {
	t.Parallel()

	server := newPreflightTestServer(models.SyncStatus{}, nil)

	preflight := server.buildPreflightStatus(context.Background(), "", "/ucdata")

	if preflight.Ready {
		t.Fatalf("preflight.Ready = true, want false")
	}

	projectCheck := findPreflightCheck(t, preflight, "project")
	if projectCheck.Status != "blocked" {
		t.Fatalf("project check status = %q, want blocked", projectCheck.Status)
	}
}

func TestBuildPreflightStatusUnavailableSharesBlocksStart(t *testing.T) {
	t.Parallel()

	server := newPreflightTestServer(models.SyncStatus{}, func(s *Server) {
		s.checkSharesAvailability = func() []syncService.UnavailableShare {
			return []syncService.UnavailableShare{{Node: "WU01", Share: "E$", Path: "/ucmount/WU01/E"}}
		}
	})

	preflight := server.buildPreflightStatus(context.Background(), "ProjA", "/ucdata")

	if preflight.Ready {
		t.Fatalf("preflight.Ready = true, want false")
	}

	sharesCheck := findPreflightCheck(t, preflight, "shares")
	if sharesCheck.Status != "blocked" {
		t.Fatalf("shares check status = %q, want blocked", sharesCheck.Status)
	}
	if len(preflight.UnavailableShares) != 1 {
		t.Fatalf("len(preflight.UnavailableShares) = %d, want 1", len(preflight.UnavailableShares))
	}
}

func TestBuildPreflightStatusLowDiskSpaceBlocksStart(t *testing.T) {
	t.Parallel()

	server := newPreflightTestServer(models.SyncStatus{}, func(s *Server) {
		s.checkDiskSpaceFunc = func(string) (syncService.DiskSpaceCheckResult, error) {
			return syncService.DiskSpaceCheckResult{
				OK:                false,
				FreeBytes:         2 * 1024 * 1024 * 1024,
				RequiredFreeBytes: 5 * 1024 * 1024 * 1024,
			}, nil
		}
	})

	preflight := server.buildPreflightStatus(context.Background(), "ProjA", "/ucdata")

	if preflight.Ready {
		t.Fatalf("preflight.Ready = true, want false")
	}

	diskCheck := findPreflightCheck(t, preflight, "disk")
	if diskCheck.Status != "blocked" {
		t.Fatalf("disk check status = %q, want blocked", diskCheck.Status)
	}
	if preflight.RequiredFreeSpaceGB <= preflight.FreeSpaceGB {
		t.Fatalf("RequiredFreeSpaceGB = %f, FreeSpaceGB = %f, want required > free", preflight.RequiredFreeSpaceGB, preflight.FreeSpaceGB)
	}
}

func TestBuildPreflightStatusRunningSyncBlocksStart(t *testing.T) {
	t.Parallel()

	server := newPreflightTestServer(models.SyncStatus{
		IsRunning:   true,
		Project:     "ProjA",
		Destination: "/ucdata",
	}, nil)

	preflight := server.buildPreflightStatus(context.Background(), "ProjA", "/ucdata")

	if preflight.Ready {
		t.Fatalf("preflight.Ready = true, want false")
	}

	syncCheck := findPreflightCheck(t, preflight, "sync")
	if syncCheck.Status != "blocked" {
		t.Fatalf("sync check status = %q, want blocked", syncCheck.Status)
	}
	if preflight.ActiveProject != "ProjA" {
		t.Fatalf("ActiveProject = %q, want ProjA", preflight.ActiveProject)
	}
}

func TestHandleGetPreflightReturnsJSON(t *testing.T) {
	t.Parallel()

	server := newPreflightTestServer(models.SyncStatus{}, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/preflight?project=ProjA&destination=%2Fucdata", nil)
	resp := httptest.NewRecorder()

	server.handleGetPreflight(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.Code)
	}

	var payload models.PreflightStatus
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if !payload.Ready {
		t.Fatalf("payload.Ready = false, want true")
	}
	if payload.SelectedProject != "ProjA" {
		t.Fatalf("SelectedProject = %q, want ProjA", payload.SelectedProject)
	}
}

func TestBuildDashboardPreflightStatusAggregatesBlockedInstance(t *testing.T) {
	t.Parallel()

	instanceA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(models.PreflightStatus{
			Ready: true,
			Checks: []models.PreflightCheck{
				{Key: "sync", Label: "Состояние службы", Status: "ready", Message: "ready"},
				{Key: "project", Label: "Проект", Status: "ready", Message: "ready"},
				{Key: "destination", Label: "Диск назначения", Status: "ready", Message: "ready"},
				{Key: "shares", Label: "Сетевые шары", Status: "ready", Message: "ready"},
				{Key: "disk", Label: "Свободное место", Status: "ready", Message: "ready"},
			},
		})
	}))
	defer instanceA.Close()

	instanceB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(models.PreflightStatus{
			Ready:             false,
			UnavailableShares: []models.PreflightUnavailableShare{{Node: "WU08", Share: "E$", Path: "/ucmount-b/WU08/E"}},
			Checks: []models.PreflightCheck{
				{Key: "sync", Label: "Состояние службы", Status: "ready", Message: "ready"},
				{Key: "project", Label: "Проект", Status: "ready", Message: "ready"},
				{Key: "destination", Label: "Диск назначения", Status: "ready", Message: "ready"},
				{Key: "shares", Label: "Сетевые шары", Status: "blocked", Message: "Недоступно сетевых шар: 1"},
				{Key: "disk", Label: "Свободное место", Status: "ready", Message: "ready"},
			},
		})
	}))
	defer instanceB.Close()

	server := &Server{
		cfg: &config.Config{Web: config.Web{Dashboard: config.WebDashboard{Instances: []config.DashboardInstance{
			{ID: "a", Name: "Instance A", URL: instanceA.URL},
			{ID: "b", Name: "Instance B", URL: instanceB.URL},
		}}}},
		httpClient: &http.Client{Timeout: 2 * time.Second},
	}

	preflight := server.buildDashboardPreflightStatus(context.Background(), "ProjA", "/ucdata")

	if preflight.Ready {
		t.Fatalf("preflight.Ready = true, want false")
	}
	if len(preflight.Instances) != 2 {
		t.Fatalf("len(preflight.Instances) = %d, want 2", len(preflight.Instances))
	}

	sharesCheck := findPreflightCheck(t, models.PreflightStatus{Checks: preflight.Checks}, "shares")
	if sharesCheck.Status != "blocked" {
		t.Fatalf("shares check status = %q, want blocked", sharesCheck.Status)
	}
	if !strings.Contains(sharesCheck.Message, "Instance B") {
		t.Fatalf("shares check message = %q, want mention Instance B", sharesCheck.Message)
	}

	if preflight.Instances[1].Ready {
		t.Fatalf("preflight.Instances[1].Ready = true, want false")
	}
	if len(preflight.Instances[1].UnavailableShares) != 1 {
		t.Fatalf("len(preflight.Instances[1].UnavailableShares) = %d, want 1", len(preflight.Instances[1].UnavailableShares))
	}
}

func TestHandleDashboardPreflightReturnsJSON(t *testing.T) {
	t.Parallel()

	instance := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/preflight" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(models.PreflightStatus{
			Ready: true,
			Checks: []models.PreflightCheck{
				{Key: "sync", Label: "Состояние службы", Status: "ready", Message: "ready"},
				{Key: "project", Label: "Проект", Status: "ready", Message: "ready"},
				{Key: "destination", Label: "Диск назначения", Status: "ready", Message: "ready"},
				{Key: "shares", Label: "Сетевые шары", Status: "ready", Message: "ready"},
				{Key: "disk", Label: "Свободное место", Status: "ready", Message: "ready"},
			},
		})
	}))
	defer instance.Close()

	server := &Server{
		cfg: &config.Config{Web: config.Web{Dashboard: config.WebDashboard{Instances: []config.DashboardInstance{{
			ID: "a", Name: "Instance A", URL: instance.URL,
		}}}}},
		httpClient: &http.Client{Timeout: 2 * time.Second},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/preflight?project=ProjA&destination=%2Fucdata", nil)
	resp := httptest.NewRecorder()

	server.handleDashboardPreflight(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.Code)
	}

	var payload models.DashboardPreflightStatus
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if !payload.Ready {
		t.Fatalf("payload.Ready = false, want true")
	}
	if payload.SelectedProject != "ProjA" {
		t.Fatalf("SelectedProject = %q, want ProjA", payload.SelectedProject)
	}
	if len(payload.Instances) != 1 {
		t.Fatalf("len(payload.Instances) = %d, want 1", len(payload.Instances))
	}
}

func newPreflightTestServer(status models.SyncStatus, mutate func(*Server)) *Server {
	server := &Server{
		cfg: &config.Config{},
		getStatusFunc: func() models.SyncStatus {
			return status
		},
		findProjectsFunc: func(context.Context) ([]models.ProjectInfo, error) {
			return []models.ProjectInfo{{Name: "ProjA", Source: "WU01/E$"}}, nil
		},
		getDestinationsFunc: func() []models.DestinationInfo {
			return []models.DestinationInfo{{Path: "/ucdata", Label: "USB-SSD Storage (default)", Type: "usb", FreeSpaceGB: 8, TotalGB: 16, IsDefault: true}}
		},
		checkSharesAvailability: func() []syncService.UnavailableShare { return nil },
		checkDiskSpaceFunc: func(string) (syncService.DiskSpaceCheckResult, error) {
			return syncService.DiskSpaceCheckResult{
				OK:                true,
				FreeBytes:         8 * 1024 * 1024 * 1024,
				RequiredFreeBytes: 1 * 1024 * 1024 * 1024,
			}, nil
		},
	}

	if mutate != nil {
		mutate(server)
	}

	return server
}

func findPreflightCheck(t *testing.T, preflight models.PreflightStatus, key string) models.PreflightCheck {
	t.Helper()

	for _, check := range preflight.Checks {
		if check.Key == key {
			return check
		}
	}

	t.Fatalf("preflight check %q not found", key)
	return models.PreflightCheck{}
}
