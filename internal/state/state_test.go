package state

import (
	"path/filepath"
	"testing"

	"github.com/zangezia/UCXSync/pkg/models"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()

	return newNamedTestStore(t, filepath.Join(t.TempDir(), "state.db"), "ucxsync-test")
}

func newNamedTestStore(t *testing.T, path, serviceName string) *Store {
	t.Helper()

	store, err := New(path, serviceName)
	if err != nil {
		t.Fatalf("failed to create test sqlite store: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("failed to close test sqlite store: %v", err)
		}
	})

	return store
}

func TestStorePersistsProjects(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	projects := []models.ProjectInfo{{Name: "ProjA", Source: "WU01/E$"}, {Name: "ProjB", Source: "WU02/F$"}}

	if err := store.SaveProjects(projects); err != nil {
		t.Fatalf("SaveProjects returned error: %v", err)
	}

	got, err := store.LoadProjects()
	if err != nil {
		t.Fatalf("LoadProjects returned error: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(got))
	}

	if got[0].Name != "ProjA" || got[1].Name != "ProjB" {
		t.Fatalf("unexpected project order/content: %#v", got)
	}
}

func TestStorePersistsCaptureProgressAndStatus(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	status, err := store.StartRun("ProjA", "/ucdata", 4)
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	if !status.IsRunning {
		t.Fatal("expected status to be marked running")
	}

	for _, fileKey := range []string{"raw:00-00", "raw:00-01", "xml:CU", "dat:CU"} {
		persisted, completed, err := store.RecordCapture(CaptureObservation{
			Project: "ProjA",
			Info: models.CaptureInfo{
				DataType:      "Lvl00",
				CaptureNumber: "00001",
				ProjectName:   "ProjA",
				SessionID:     "ABC_DEF",
				IsVerified:    true,
			},
			FileKey:          fileKey,
			RequiredRawFiles: 2,
			RequireXML:       true,
			RequireDAT:       true,
		})
		if err != nil {
			t.Fatalf("RecordCapture returned error: %v", err)
		}
		if fileKey != "dat:CU" && completed {
			t.Fatalf("capture should not complete before final file, fileKey=%s", fileKey)
		}
		if fileKey == "dat:CU" {
			if !completed {
				t.Fatal("expected capture to complete after RawQv")
			}
			if persisted.CompletedCaptures != 1 {
				t.Fatalf("expected 1 completed capture, got %d", persisted.CompletedCaptures)
			}
			if persisted.LastCaptureNumber != "00001" {
				t.Fatalf("expected last capture number 00001, got %q", persisted.LastCaptureNumber)
			}
		}
	}

	loaded, err := store.LoadStatus()
	if err != nil {
		t.Fatalf("LoadStatus returned error: %v", err)
	}

	if loaded.CompletedCaptures != 1 {
		t.Fatalf("expected persisted completed captures=1, got %d", loaded.CompletedCaptures)
	}
}

func TestStoreAggregatesCaptureAcrossServices(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "shared-state.db")
	storeA := newNamedTestStore(t, path, "ucxsync@a")
	storeB := newNamedTestStore(t, path, "ucxsync@b")

	if _, err := storeA.StartRun("ProjA", "/ucdata", 4); err != nil {
		t.Fatalf("storeA StartRun returned error: %v", err)
	}
	if _, err := storeB.StartRun("ProjA", "/ucdata", 4); err != nil {
		t.Fatalf("storeB StartRun returned error: %v", err)
	}

	steps := []struct {
		store   *Store
		fileKey string
		sensor  string
	}{
		{store: storeA, fileKey: "raw:00-00", sensor: "00-00"},
		{store: storeB, fileKey: "raw:00-01", sensor: "00-01"},
		{store: storeA, fileKey: "xml:CU"},
		{store: storeB, fileKey: "dat:CU"},
	}

	for _, step := range steps {
		_, _, err := step.store.RecordCapture(CaptureObservation{
			Project: "ProjA",
			Info: models.CaptureInfo{
				DataType:      "Lvl00",
				CaptureNumber: "00002",
				ProjectName:   "ProjA",
				SensorCode:    step.sensor,
				SessionID:     "ABC_DEF",
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

	statusA, err := storeA.LoadStatus()
	if err != nil {
		t.Fatalf("storeA LoadStatus returned error: %v", err)
	}
	statusB, err := storeB.LoadStatus()
	if err != nil {
		t.Fatalf("storeB LoadStatus returned error: %v", err)
	}

	if statusA.CompletedCaptures != 1 || statusB.CompletedCaptures != 1 {
		t.Fatalf("expected shared completed capture count 1, got A=%d B=%d", statusA.CompletedCaptures, statusB.CompletedCaptures)
	}
}
