package report

import (
	"testing"
	"time"

	"github.com/zangezia/UCXSync/internal/state"
)

func TestBuildKeepsShareProjectSeparateFromEADProjectName(t *testing.T) {
	t.Parallel()

	capturedAt := time.Date(2025, 9, 3, 4, 54, 31, 0, time.UTC)
	report := Build("ShareProjA", []state.EADRecord{
		{
			ProjectName:     "ShareProjA",
			CaptureNumber:   "00027",
			EADProjectName:  "FlightProject-42",
			Area:            "Flight-Area",
			LineNumber:      19,
			WaypointNumber:  8,
			ExposureNumber:  27,
			CapturedAt:      capturedAt,
			Latitude:        59.27014,
			Longitude:       37.25717,
			Altitude:        3438.5,
			TrackOverGround: 200,
		},
	})

	if report.Project != "ShareProjA" {
		t.Fatalf("report.Project = %q, want ShareProjA", report.Project)
	}
	if report.RecordCount != 1 || len(report.Exposures) != 1 {
		t.Fatalf("unexpected report sizing: count=%d exposures=%d", report.RecordCount, len(report.Exposures))
	}
	exposure := report.Exposures[0]
	if exposure.ProjectName != "FlightProject-42" {
		t.Fatalf("exposure.ProjectName = %q, want FlightProject-42", exposure.ProjectName)
	}
	if exposure.ProjectName == report.Project {
		t.Fatalf("expected EAD project_name to remain distinct from share project name; both are %q", exposure.ProjectName)
	}
	if exposure.Date != "250903" || exposure.Time != "045431" {
		t.Fatalf("unexpected timestamp projection: date=%q time=%q", exposure.Date, exposure.Time)
	}
}
