package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/zangezia/UCXSync/internal/state"
)

type Exposure struct {
	CaptureNumber  string `json:"capture_number"`
	ExposureNumber int    `json:"exposure_number"`
	// ProjectName is copied from the EAD XML <project_name> field and may differ
	// from the share/destination project name used to group the report file.
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
}

type DestinationReport struct {
	// Project is the project name from the shares/destination context. It is used
	// for report grouping/pathing and is intentionally distinct from exposure
	// project_name values parsed from the EAD payload.
	Project     string     `json:"project"`
	GeneratedAt time.Time  `json:"generated_at"`
	RecordCount int        `json:"record_count"`
	Exposures   []Exposure `json:"exposures"`
}

func Build(shareProject string, records []state.EADRecord) DestinationReport {
	exposures := make([]Exposure, 0, len(records))
	for _, record := range records {
		exposures = append(exposures, Exposure{
			CaptureNumber:   record.CaptureNumber,
			ExposureNumber:  record.ExposureNumber,
			ProjectName:     record.EADProjectName,
			Area:            record.Area,
			LineNumber:      record.LineNumber,
			WaypointNumber:  record.WaypointNumber,
			Date:            record.CapturedAt.UTC().Format("060102"),
			Time:            record.CapturedAt.UTC().Format("150405"),
			Latitude:        record.Latitude,
			Longitude:       record.Longitude,
			Altitude:        record.Altitude,
			TrackOverGround: record.TrackOverGround,
		})
	}

	return DestinationReport{
		Project:     shareProject,
		GeneratedAt: time.Now().UTC(),
		RecordCount: len(exposures),
		Exposures:   exposures,
	}
}

func DefaultPath(destinationRoot, project string) string {
	return filepath.Join(destinationRoot, fmt.Sprintf("%s-ead-report.json", project))
}

func WriteJSON(path string, payload DestinationReport) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	tmpFile, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	encoder := json.NewEncoder(tmpFile)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(payload); err != nil {
		tmpFile.Close()
		return err
	}
	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}

	if err := os.Rename(tmpPath, path); err != nil {
		if removeErr := os.Remove(path); removeErr != nil && !os.IsNotExist(removeErr) {
			return err
		}
		if retryErr := os.Rename(tmpPath, path); retryErr != nil {
			return retryErr
		}
	}

	return nil
}
