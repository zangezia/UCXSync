package state

import (
	"database/sql"
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

func TestStoreSaveEADProcessingIsIdempotent(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	modTime := time.Unix(1710001111, 0).UTC()
	agl := 3310.8

	record := EADRecord{
		ProjectName:         "ProjA",
		RelativePath:        "ead/EAD-00027-ProjA-ABC.xml",
		CaptureNumber:       "00027",
		EADProjectName:      "Vologda_2k",
		Area:                "Cherepovec",
		RecordGUID:          "FF4070C7-B7E0-40E5-B7F3-F8C00FD4AFE4",
		SessionID:           "FF4070C7_B7E0_40E5_B7F3_F8C00FD4AFE4",
		LineNumber:          19,
		SegmentNumber:       1,
		WaypointNumber:      8,
		ExposureNumber:      27,
		CapturedAt:          time.Date(2025, 9, 3, 4, 54, 31, 0, time.UTC),
		Latitude:            59.27014,
		Longitude:           37.25717,
		Altitude:            3438.5,
		AboveGroundLevel:    &agl,
		TrackOverGround:     200,
		GroundSpeed:         111.1,
		Software:            "COSa V4.5.5",
		Aperture:            "F 8",
		ExposureTimeSeconds: 0.002,
	}
	status := EADProcessingStatus{
		ProjectName:  record.ProjectName,
		RelativePath: record.RelativePath,
		FileSize:     1234,
		ModTime:      modTime,
		Status:       "success",
	}

	if err := store.SaveEADProcessing(record, status); err != nil {
		t.Fatalf("SaveEADProcessing returned error: %v", err)
	}

	record.GroundSpeed = 112.2
	status.WarningMessage = "minor normalization note"
	if err := store.SaveEADProcessing(record, status); err != nil {
		t.Fatalf("second SaveEADProcessing returned error: %v", err)
	}

	loadedRecord, found, err := store.LoadEADRecord("ProjA", "ead/EAD-00027-ProjA-ABC.xml")
	if err != nil {
		t.Fatalf("LoadEADRecord returned error: %v", err)
	}
	if !found {
		t.Fatal("expected EAD record to exist")
	}
	if loadedRecord.GroundSpeed != 112.2 {
		t.Fatalf("GroundSpeed = %v, want 112.2", loadedRecord.GroundSpeed)
	}
	if loadedRecord.Area != "Cherepovec" {
		t.Fatalf("Area = %q, want Cherepovec", loadedRecord.Area)
	}

	loadedStatus, found, err := store.LoadEADProcessingStatus("ProjA", "ead/EAD-00027-ProjA-ABC.xml")
	if err != nil {
		t.Fatalf("LoadEADProcessingStatus returned error: %v", err)
	}
	if !found {
		t.Fatal("expected EAD processing status to exist")
	}
	if loadedStatus.WarningMessage != "minor normalization note" {
		t.Fatalf("WarningMessage = %q, want updated warning", loadedStatus.WarningMessage)
	}

	var recordCount, statusCount int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM ead_records WHERE project_name = ? AND relative_path = ?`, "ProjA", "ead/EAD-00027-ProjA-ABC.xml").Scan(&recordCount); err != nil {
		t.Fatalf("count ead_records failed: %v", err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM ead_processing_status WHERE project_name = ? AND relative_path = ?`, "ProjA", "ead/EAD-00027-ProjA-ABC.xml").Scan(&statusCount); err != nil {
		t.Fatalf("count ead_processing_status failed: %v", err)
	}
	if recordCount != 1 || statusCount != 1 {
		t.Fatalf("expected single upserted rows, got records=%d status=%d", recordCount, statusCount)
	}
}

func TestStoreRecordsEADProcessingFailure(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	modTime := time.Unix(1710002222, 0).UTC()
	relativePath := "ead/bad.xml"

	if err := store.SaveEADProcessing(EADRecord{
		ProjectName:         "ProjA",
		RelativePath:        relativePath,
		CaptureNumber:       "00027",
		EADProjectName:      "ProjA",
		RecordGUID:          "ABC",
		SessionID:           "ABC",
		LineNumber:          1,
		SegmentNumber:       1,
		WaypointNumber:      1,
		ExposureNumber:      27,
		CapturedAt:          time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC),
		Latitude:            1,
		Longitude:           2,
		Altitude:            3,
		TrackOverGround:     4,
		GroundSpeed:         5,
		Software:            "COSa",
		Aperture:            "F 8",
		ExposureTimeSeconds: 0.001,
	}, EADProcessingStatus{
		ProjectName:  "ProjA",
		RelativePath: relativePath,
		FileSize:     100,
		ModTime:      modTime,
		Status:       "success",
	}); err != nil {
		t.Fatalf("SaveEADProcessing returned error: %v", err)
	}

	if err := store.RecordEADProcessingFailure(EADProcessingStatus{
		ProjectName:    "ProjA",
		RelativePath:   relativePath,
		FileSize:       77,
		ModTime:        modTime,
		Status:         "error",
		ErrorMessage:   "ead parse error at line 3, column 17: unexpected EOF",
		WarningMessage: "truncated file",
	}); err != nil {
		t.Fatalf("RecordEADProcessingFailure returned error: %v", err)
	}

	loadedStatus, found, err := store.LoadEADProcessingStatus("ProjA", "ead/bad.xml")
	if err != nil {
		t.Fatalf("LoadEADProcessingStatus returned error: %v", err)
	}
	if !found {
		t.Fatal("expected failure status to exist")
	}
	if loadedStatus.Status != "error" {
		t.Fatalf("Status = %q, want error", loadedStatus.Status)
	}
	if loadedStatus.ErrorMessage == "" {
		t.Fatal("expected error message to be stored")
	}

	_, found, err = store.LoadEADRecord("ProjA", "ead/bad.xml")
	if err != nil {
		t.Fatalf("LoadEADRecord returned error: %v", err)
	}
	if found {
		t.Fatal("did not expect successful EAD record for failed processing")
	}
}

func TestStoreSaveEADProcessingRollsBackOnStatusInsertFailure(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)

	err := store.SaveEADProcessing(
		EADRecord{
			ProjectName:         "ProjA",
			RelativePath:        "ead/good.xml",
			CaptureNumber:       "00001",
			EADProjectName:      "ProjA",
			RecordGUID:          "ABC",
			SessionID:           "ABC",
			LineNumber:          1,
			SegmentNumber:       1,
			WaypointNumber:      1,
			ExposureNumber:      1,
			CapturedAt:          time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC),
			Latitude:            1,
			Longitude:           2,
			Altitude:            3,
			TrackOverGround:     4,
			GroundSpeed:         5,
			Software:            "COSa",
			Aperture:            "F 8",
			ExposureTimeSeconds: 0.001,
		},
		EADProcessingStatus{
			ProjectName:  "ProjA",
			RelativePath: "",
			FileSize:     123,
			ModTime:      time.Unix(1710003333, 0).UTC(),
			Status:       "success",
		},
	)
	if err == nil {
		t.Fatal("expected SaveEADProcessing to fail when status relative path is empty")
	}

	_, found, err := store.LoadEADRecord("ProjA", "ead/good.xml")
	if err != nil && err != sql.ErrNoRows {
		t.Fatalf("LoadEADRecord returned unexpected error: %v", err)
	}
	if found {
		t.Fatal("expected EAD record insert to be rolled back")
	}
}

func TestStoreListCompletedEADRecordsReturnsCompletedCapturesOnly(t *testing.T) {
	t.Parallel()

	store := newTestStore(t)
	if _, err := store.StartRun("ProjA", "/ucdata", 4); err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}

	completedRecord := EADRecord{
		ProjectName:         "ProjA",
		RelativePath:        "EAD-00027-ProjA-ABC.xml",
		CaptureNumber:       "00027",
		EADProjectName:      "ProjA",
		Area:                "Area-27",
		RecordGUID:          "GUID-27",
		SessionID:           "GUID_27",
		LineNumber:          10,
		SegmentNumber:       1,
		WaypointNumber:      5,
		ExposureNumber:      27,
		CapturedAt:          time.Date(2025, 9, 3, 4, 54, 31, 0, time.UTC),
		Latitude:            59.1,
		Longitude:           37.2,
		Altitude:            1000,
		TrackOverGround:     180,
		GroundSpeed:         120,
		Software:            "COSa",
		Aperture:            "F 8",
		ExposureTimeSeconds: 0.002,
	}
	pendingRecord := EADRecord{
		ProjectName:         "ProjA",
		RelativePath:        "EAD-00028-ProjA-ABC.xml",
		CaptureNumber:       "00028",
		EADProjectName:      "ProjA",
		Area:                "Area-28",
		RecordGUID:          "GUID-28",
		SessionID:           "GUID_28",
		LineNumber:          11,
		SegmentNumber:       1,
		WaypointNumber:      6,
		ExposureNumber:      28,
		CapturedAt:          time.Date(2025, 9, 3, 4, 55, 31, 0, time.UTC),
		Latitude:            59.2,
		Longitude:           37.3,
		Altitude:            1001,
		TrackOverGround:     181,
		GroundSpeed:         121,
		Software:            "COSa",
		Aperture:            "F 8",
		ExposureTimeSeconds: 0.002,
	}
	for _, record := range []EADRecord{completedRecord, pendingRecord} {
		if err := store.SaveEADProcessing(record, EADProcessingStatus{
			ProjectName:  record.ProjectName,
			RelativePath: record.RelativePath,
			FileSize:     100,
			ModTime:      record.CapturedAt,
			Status:       "success",
		}); err != nil {
			t.Fatalf("SaveEADProcessing(%s) returned error: %v", record.CaptureNumber, err)
		}
	}

	for _, obs := range []CaptureObservation{
		{
			Project: "ProjA",
			Info:    models.CaptureInfo{DataType: "Lvl00", CaptureNumber: "00027", ProjectName: "ProjA", SensorCode: "00-00", SessionID: "GUID_27", IsVerified: true},
			FileKey: "raw:00-00", RequiredRawFiles: 1, RequireXML: true, RequireDAT: true,
		},
		{
			Project: "ProjA",
			Info:    models.CaptureInfo{DataType: "EAD", CaptureNumber: "00027", ProjectName: "ProjA", SessionID: "GUID_27", IsVerified: true},
			FileKey: "xml:CU", RequiredRawFiles: 1, RequireXML: true, RequireDAT: true,
		},
		{
			Project: "ProjA",
			Info:    models.CaptureInfo{DataType: "RawQv", CaptureNumber: "00027", ProjectName: "ProjA", SessionID: "GUID_27", IsVerified: true},
			FileKey: "dat:CU", RequiredRawFiles: 1, RequireXML: true, RequireDAT: true,
		},
		{
			Project: "ProjA",
			Info:    models.CaptureInfo{DataType: "Lvl00", CaptureNumber: "00028", ProjectName: "ProjA", SensorCode: "00-00", SessionID: "GUID_28", IsVerified: true},
			FileKey: "raw:00-00", RequiredRawFiles: 1, RequireXML: true, RequireDAT: true,
		},
	} {
		if _, _, err := store.RecordCapture(obs); err != nil {
			t.Fatalf("RecordCapture(%s/%s) returned error: %v", obs.Info.CaptureNumber, obs.FileKey, err)
		}
	}

	records, err := store.ListCompletedEADRecords("ProjA")
	if err != nil {
		t.Fatalf("ListCompletedEADRecords returned error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 completed EAD record, got %d", len(records))
	}
	if records[0].CaptureNumber != "00027" {
		t.Fatalf("CaptureNumber = %q, want 00027", records[0].CaptureNumber)
	}
	if records[0].Area != "Area-27" {
		t.Fatalf("Area = %q, want Area-27", records[0].Area)
	}
}
