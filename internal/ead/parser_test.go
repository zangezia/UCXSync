package ead

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestParseFileExtractsExpectedFields(t *testing.T) {
	t.Parallel()

	path := filepath.Join("testdata", "valid_ead.xml")
	record, issues, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile returned error: %v", err)
	}
	if len(issues) != 0 {
		t.Fatalf("expected no warnings, got %#v", issues)
	}

	if record.CaptureNumber != "00027" {
		t.Fatalf("CaptureNumber = %q, want 00027", record.CaptureNumber)
	}
	if record.ProjectName != "Vologda_2k" {
		t.Fatalf("ProjectName = %q, want Vologda_2k", record.ProjectName)
	}
	if record.Area != "Cherepovec" {
		t.Fatalf("Area = %q, want Cherepovec", record.Area)
	}
	if record.RecordGUID != "FF4070C7-B7E0-40E5-B7F3-F8C00FD4AFE4" {
		t.Fatalf("RecordGUID = %q", record.RecordGUID)
	}
	if record.SessionID != "FF4070C7_B7E0_40E5_B7F3_F8C00FD4AFE4" {
		t.Fatalf("SessionID = %q", record.SessionID)
	}
	if record.LineNumber != 19 || record.SegmentNumber != 1 || record.WaypointNumber != 8 || record.ExposureNumber != 27 {
		t.Fatalf("unexpected identifiers: %+v", record)
	}
	if record.CapturedAt.UTC().Format("2006-01-02T15:04:05Z") != "2025-09-03T04:54:31Z" {
		t.Fatalf("CapturedAt = %s, want 2025-09-03T04:54:31Z", record.CapturedAt.UTC().Format("2006-01-02T15:04:05Z"))
	}
	if record.Latitude != 59.27014 || record.Longitude != 37.25717 {
		t.Fatalf("unexpected coordinates: lat=%v lon=%v", record.Latitude, record.Longitude)
	}
	if record.Altitude != 3438.5 {
		t.Fatalf("Altitude = %v, want 3438.5", record.Altitude)
	}
	if record.AboveGroundLevel == nil || *record.AboveGroundLevel != 3310.8 {
		t.Fatalf("AboveGroundLevel = %v, want 3310.8", record.AboveGroundLevel)
	}
	if record.TrackOverGround != 200 || record.GroundSpeed != 111.1 {
		t.Fatalf("unexpected navigation data: track=%v speed=%v", record.TrackOverGround, record.GroundSpeed)
	}
	if record.Software != "COSa V4.5.5" {
		t.Fatalf("Software = %q, want COSa V4.5.5", record.Software)
	}
	if record.Aperture != "F 8" {
		t.Fatalf("Aperture = %q, want F 8", record.Aperture)
	}
	if record.ExposureTimeSeconds != 0.002 {
		t.Fatalf("ExposureTimeSeconds = %v, want 0.002", record.ExposureTimeSeconds)
	}
}

func TestParseAllowsOptionalAboveGroundLevelOmission(t *testing.T) {
	t.Parallel()

	xmlData := []byte(`<?xml version="1.0" encoding="utf-8"?>
<exposure_annotation_data>
	<image_number>27</image_number>
	<record_guid>FF4070C7-B7E0-40E5-B7F3-F8C00FD4AFE4</record_guid>
	<software>COSa V4.5.5</software>
	<aperture description="F 8">1</aperture>
	<exposure_time>0.002</exposure_time>
	<exposure_annotation_info>
		<fms_info>
			<exposure_number>27</exposure_number>
			<project_name>Vologda_2k</project_name>
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
</exposure_annotation_data>`)

	record, issues, err := Parse(xmlData, "EAD-00027-ProjA-ABC.xml")
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if record.AboveGroundLevel != nil {
		t.Fatalf("expected nil AboveGroundLevel, got %v", *record.AboveGroundLevel)
	}
	if len(issues) != 1 {
		t.Fatalf("expected single warning for missing AGL, got %#v", issues)
	}
}

func TestParseMalformedXMLReturnsStructuredError(t *testing.T) {
	t.Parallel()

	path := filepath.Join("testdata", "malformed_ead.xml")
	_, _, err := ParseFile(path)
	if err == nil {
		t.Fatal("expected ParseFile to fail for malformed XML")
	}

	var parseErr *ParseError
	if !errors.As(err, &parseErr) {
		t.Fatalf("expected ParseError, got %T", err)
	}
	if parseErr.Path == "" {
		t.Fatal("expected ParseError to include source path")
	}
	if parseErr.Line == 0 {
		t.Fatal("expected ParseError to include line information")
	}
}
