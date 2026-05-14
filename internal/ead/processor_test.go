package ead

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/zangezia/UCXSync/internal/state"
	syncservice "github.com/zangezia/UCXSync/internal/sync"
	"github.com/zangezia/UCXSync/pkg/models"
)

func TestProcessorWritesDestinationReportForCompletedCapture(t *testing.T) {
	t.Parallel()

	baseDir := t.TempDir()
	store, err := state.New(filepath.Join(baseDir, "state.db"), "ucxsync-test")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	if _, err := store.StartRun("ShareProjA", filepath.Join(baseDir, "dest"), 1); err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}

	for _, obs := range []state.CaptureObservation{
		{
			Project: "ShareProjA",
			Info:    models.CaptureInfo{DataType: "Lvl00", CaptureNumber: "00027", ProjectName: "ShareProjA", SensorCode: "00-00", SessionID: "FF4070C7_B7E0_40E5_B7F3_F8C00FD4AFE4", IsVerified: true},
			FileKey: "raw:00-00", RequiredRawFiles: 1, RequireXML: true, RequireDAT: true,
		},
		{
			Project: "ShareProjA",
			Info:    models.CaptureInfo{DataType: "EAD", CaptureNumber: "00027", ProjectName: "ShareProjA", SessionID: "FF4070C7_B7E0_40E5_B7F3_F8C00FD4AFE4", IsVerified: true},
			FileKey: "xml:CU", RequiredRawFiles: 1, RequireXML: true, RequireDAT: true,
		},
		{
			Project: "ShareProjA",
			Info:    models.CaptureInfo{DataType: "RawQv", CaptureNumber: "00027", ProjectName: "ShareProjA", SessionID: "FF4070C7_B7E0_40E5_B7F3_F8C00FD4AFE4", IsVerified: true},
			FileKey: "dat:CU", RequiredRawFiles: 1, RequireXML: true, RequireDAT: true,
		},
	} {
		if _, _, err := store.RecordCapture(obs); err != nil {
			t.Fatalf("RecordCapture(%s) returned error: %v", obs.FileKey, err)
		}
	}

	destinationRoot := filepath.Join(baseDir, "dest")
	if err := os.MkdirAll(destinationRoot, 0755); err != nil {
		t.Fatalf("failed to create destination root: %v", err)
	}
	eadPath := filepath.Join(destinationRoot, "EAD-00027-ShareProjA-FF4070C7_B7E0_40E5_B7F3_F8C00FD4AFE4.xml")
	if err := os.WriteFile(eadPath, []byte(`<?xml version="1.0" encoding="utf-8"?>
<exposure_annotation_data>
	<image_number>27</image_number>
	<record_guid>FF4070C7-B7E0-40E5-B7F3-F8C00FD4AFE4</record_guid>
	<software>COSa V4.5.5</software>
	<aperture description="F 8">1</aperture>
	<exposure_time>0.002</exposure_time>
	<exposure_annotation_info>
		<fms_info>
			<exposure_number>27</exposure_number>
			<project_name>FlightProject-42</project_name>
			<area>Flight-Area</area>
			<line_number>19</line_number>
			<segment_number>1</segment_number>
			<waypoint_number>8</waypoint_number>
		</fms_info>
		<gps_navigation_info>
			<date>250903</date>
			<time>045431</time>
			<latitude>N59.270140</latitude>
			<longitude>E037.257170</longitude>
			<altitude>3438.5</altitude>
			<track_over_ground>200</track_over_ground>
			<ground_speed>111.1</ground_speed>
		</gps_navigation_info>
	</exposure_annotation_info>
</exposure_annotation_data>`), 0644); err != nil {
		t.Fatalf("failed to write EAD fixture: %v", err)
	}

	processor := NewProcessor(store)
	event := syncservice.CopiedFileEvent{
		Project:         "ShareProjA",
		RelativePath:    filepath.Base(eadPath),
		DestinationPath: eadPath,
		DestinationRoot: destinationRoot,
		FileSize:        1,
	}
	if err := processor.ProcessCopiedFile(nil, event); err != nil {
		t.Fatalf("ProcessCopiedFile returned error: %v", err)
	}

	reportPath := filepath.Join(destinationRoot, "ShareProjA-ead-report.json")
	reportData, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("failed to read generated report: %v", err)
	}

	var payload struct {
		Project     string `json:"project"`
		RecordCount int    `json:"record_count"`
		Exposures   []struct {
			CaptureNumber   string  `json:"capture_number"`
			ExposureNumber  int     `json:"exposure_number"`
			ProjectName     string  `json:"project_name"`
			Area            string  `json:"area"`
			LineNumber      int     `json:"line_number"`
			WaypointNumber  int     `json:"waypoint_number"`
			Date            string  `json:"date"`
			Time            string  `json:"time"`
			Latitude        float64 `json:"latitude"`
			Longitude       float64 `json:"longitude"`
			Altitude        float64 `json:"altitude"`
			TrackOverGround float64 `json:"track_over_ground"`
		} `json:"exposures"`
	}
	if err := json.Unmarshal(reportData, &payload); err != nil {
		t.Fatalf("failed to unmarshal generated report: %v", err)
	}
	if payload.Project != "ShareProjA" || payload.RecordCount != 1 || len(payload.Exposures) != 1 {
		t.Fatalf("unexpected report header: %#v", payload)
	}
	exposure := payload.Exposures[0]
	if exposure.CaptureNumber != "00027" || exposure.ExposureNumber != 27 {
		t.Fatalf("unexpected capture/exposure identifiers: %#v", exposure)
	}
	if exposure.ProjectName != "FlightProject-42" || exposure.Area != "Flight-Area" {
		t.Fatalf("unexpected project/area fields: %#v", exposure)
	}
	if exposure.ProjectName == payload.Project {
		t.Fatalf("expected exposure project_name to differ from share project when EAD says so: %#v", exposure)
	}
	if exposure.LineNumber != 19 || exposure.WaypointNumber != 8 {
		t.Fatalf("unexpected line/waypoint fields: %#v", exposure)
	}
	if exposure.Date != "250903" || exposure.Time != "045431" {
		t.Fatalf("unexpected date/time fields: %#v", exposure)
	}
	if exposure.Latitude != 59.27014 || exposure.Longitude != 37.25717 {
		t.Fatalf("unexpected coordinates: %#v", exposure)
	}
	if exposure.Altitude != 3438.5 || exposure.TrackOverGround != 200 {
		t.Fatalf("unexpected altitude/track fields: %#v", exposure)
	}
}
