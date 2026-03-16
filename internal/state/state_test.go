package state

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

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

func TestStoreConcurrentWritesSharedDatabase(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "shared-state.db")
	storeA := newNamedTestStore(t, path, "ucxsync@a")
	storeB := newNamedTestStore(t, path, "ucxsync@b")

	if _, err := storeA.StartRun("ProjBusy", "/ucdata", 4); err != nil {
		t.Fatalf("storeA StartRun returned error: %v", err)
	}
	if _, err := storeB.StartRun("ProjBusy", "/ucdata", 4); err != nil {
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

	var wg sync.WaitGroup
	errs := make(chan error, len(steps))
	for _, step := range steps {
		step := step
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := step.store.RecordCapture(CaptureObservation{
				Project: "ProjBusy",
				Info: models.CaptureInfo{
					DataType:      "Lvl00",
					CaptureNumber: "00077",
					ProjectName:   "ProjBusy",
					SensorCode:    step.sensor,
					SessionID:     "BUSY_TEST",
					IsVerified:    true,
				},
				FileKey:          step.fileKey,
				RequiredRawFiles: 2,
				RequireXML:       true,
				RequireDAT:       true,
			})
			errs <- err
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent RecordCapture returned error: %v", err)
		}
	}

	statusA, err := storeA.LoadStatus()
	if err != nil {
		t.Fatalf("storeA LoadStatus returned error: %v", err)
	}
	if statusA.CompletedCaptures != 1 {
		t.Fatalf("expected completed capture count 1 after concurrent writes, got %d", statusA.CompletedCaptures)
	}
}

func TestStoreTracksCopiedFiles(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	modTime := time.Unix(1710000000, 123).UTC()

	marked, err := store.IsFileCopied("ProjA", "raw/file.raw", 100, modTime)
	if err != nil {
		t.Fatalf("IsFileCopied returned error: %v", err)
	}
	if marked {
		t.Fatal("expected file to be absent before mark")
	}

	if err := store.MarkFileCopied("ProjA", "raw/file.raw", 100, modTime); err != nil {
		t.Fatalf("MarkFileCopied returned error: %v", err)
	}

	marked, err = store.IsFileCopied("ProjA", "raw/file.raw", 100, modTime)
	if err != nil {
		t.Fatalf("IsFileCopied after mark returned error: %v", err)
	}
	if !marked {
		t.Fatal("expected file to be marked copied")
	}

	marked, err = store.IsFileCopied("ProjA", "raw/file.raw", 101, modTime)
	if err != nil {
		t.Fatalf("IsFileCopied with different size returned error: %v", err)
	}
	if marked {
		t.Fatal("expected different file size to invalidate copied mark")
	}

	if err := store.ResetCopiedFiles("ProjA"); err != nil {
		t.Fatalf("ResetCopiedFiles returned error: %v", err)
	}

	marked, err = store.IsFileCopied("ProjA", "raw/file.raw", 100, modTime)
	if err != nil {
		t.Fatalf("IsFileCopied after reset returned error: %v", err)
	}
	if marked {
		t.Fatal("expected copied mark to be removed after reset")
	}
}

func TestStorePromotesCaptureToTestWhenRawArrivesAfterMetadata(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	if _, err := store.StartRun("ProjTest", "/ucdata", 4); err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}

	steps := []CaptureObservation{
		{
			Project: "ProjTest",
			Info: models.CaptureInfo{
				DataType:      "EAD",
				CaptureNumber: "00042",
				ProjectName:   "ProjTest",
				SessionID:     "SESSION_TEST",
				IsTest:        false,
			},
			FileKey:          "xml:CU",
			RequiredRawFiles: 2,
			RequireXML:       true,
			RequireDAT:       true,
		},
		{
			Project: "ProjTest",
			Info: models.CaptureInfo{
				DataType:      "Lvl0X",
				CaptureNumber: "00042",
				ProjectName:   "ProjTest",
				SensorCode:    "00-00",
				SessionID:     "SESSION_TEST",
				IsVerified:    false,
				IsTest:        true,
			},
			FileKey:          "raw:00-00",
			RequiredRawFiles: 2,
			RequireXML:       true,
			RequireDAT:       true,
		},
		{
			Project: "ProjTest",
			Info: models.CaptureInfo{
				DataType:      "Lvl0X",
				CaptureNumber: "00042",
				ProjectName:   "ProjTest",
				SensorCode:    "00-01",
				SessionID:     "SESSION_TEST",
				IsVerified:    false,
				IsTest:        true,
			},
			FileKey:          "raw:00-01",
			RequiredRawFiles: 2,
			RequireXML:       true,
			RequireDAT:       true,
		},
		{
			Project: "ProjTest",
			Info: models.CaptureInfo{
				DataType:      "RawQv",
				CaptureNumber: "00042",
				ProjectName:   "ProjTest",
				SessionID:     "SESSION_TEST",
				IsTest:        false,
			},
			FileKey:          "dat:CU",
			RequiredRawFiles: 2,
			RequireXML:       true,
			RequireDAT:       true,
		},
	}

	var persisted models.PersistedCaptureStatus
	for _, step := range steps {
		var err error
		persisted, _, err = store.RecordCapture(step)
		if err != nil {
			t.Fatalf("RecordCapture(%s) returned error: %v", step.FileKey, err)
		}
	}

	if persisted.CompletedTestCaptures != 1 {
		t.Fatalf("CompletedTestCaptures = %d, want 1", persisted.CompletedTestCaptures)
	}
	if persisted.LastTestCaptureNumber != "00042" {
		t.Fatalf("LastTestCaptureNumber = %q, want 00042", persisted.LastTestCaptureNumber)
	}
	if persisted.CompletedCaptures != 0 {
		t.Fatalf("CompletedCaptures = %d, want 0", persisted.CompletedCaptures)
	}

	loaded, err := store.LoadProjectStatus("ProjTest")
	if err != nil {
		t.Fatalf("LoadProjectStatus returned error: %v", err)
	}
	if loaded.CompletedTestCaptures != 1 {
		t.Fatalf("LoadProjectStatus CompletedTestCaptures = %d, want 1", loaded.CompletedTestCaptures)
	}
}
