package ead

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/zangezia/UCXSync/internal/report"
	"github.com/zangezia/UCXSync/internal/state"
	syncservice "github.com/zangezia/UCXSync/internal/sync"
)

var (
	eadMetadataPathRegex = regexp.MustCompile(`(?i)^EAD-(\d+)(?:-T)?-.*\.xml$`)
	rawCapturePathRegex  = regexp.MustCompile(`(?i)^Lvl\d+X?-(\d+)(?:-(T))?-(.+)-(\d+-\d+)-([A-F0-9_]+)\.raw$`)
	rawQvPathRegex       = regexp.MustCompile(`(?i)^RawQv-(\d+)(?:-(T))?-(.+)-([A-F0-9_]+)\.dat$`)
)

type Processor struct {
	store *state.Store
}

func NewProcessor(store *state.Store) *Processor {
	return &Processor{store: store}
}

func (p *Processor) ProcessCopiedFile(_ context.Context, event syncservice.CopiedFileEvent) error {
	if p == nil || p.store == nil {
		return nil
	}

	var processingErr error
	if isEADMetadataFile(event.RelativePath) {
		record, issues, err := ParseFile(event.DestinationPath)
		if err != nil {
			statusErr := p.store.RecordEADProcessingFailure(state.EADProcessingStatus{
				ProjectName:    event.Project,
				RelativePath:   event.RelativePath,
				FileSize:       event.FileSize,
				ModTime:        event.ModTime,
				Status:         "error",
				WarningMessage: joinIssues(issues),
				ErrorMessage:   err.Error(),
			})
			if statusErr != nil {
				return fmt.Errorf("persist EAD failure status: %w (original error: %v)", statusErr, err)
			}
			processingErr = err
		} else {
			if err := p.store.SaveEADProcessing(
				state.EADRecord{
					ProjectName:         event.Project,
					RelativePath:        event.RelativePath,
					CaptureNumber:       record.CaptureNumber,
					EADProjectName:      record.ProjectName,
					Area:                record.Area,
					RecordGUID:          record.RecordGUID,
					SessionID:           record.SessionID,
					LineNumber:          record.LineNumber,
					SegmentNumber:       record.SegmentNumber,
					WaypointNumber:      record.WaypointNumber,
					ExposureNumber:      record.ExposureNumber,
					CapturedAt:          record.CapturedAt,
					Latitude:            record.Latitude,
					Longitude:           record.Longitude,
					Altitude:            record.Altitude,
					AboveGroundLevel:    record.AboveGroundLevel,
					TrackOverGround:     record.TrackOverGround,
					GroundSpeed:         record.GroundSpeed,
					Software:            record.Software,
					Aperture:            record.Aperture,
					ExposureTimeSeconds: record.ExposureTimeSeconds,
				},
				state.EADProcessingStatus{
					ProjectName:    event.Project,
					RelativePath:   event.RelativePath,
					FileSize:       event.FileSize,
					ModTime:        event.ModTime,
					Status:         "success",
					WarningMessage: joinIssues(issues),
				},
			); err != nil {
				return err
			}
		}
	}

	captureNumber := parseCaptureNumber(event.RelativePath)
	if captureNumber == "" || strings.TrimSpace(event.DestinationRoot) == "" {
		return processingErr
	}

	completed, err := p.store.IsCaptureDone(event.Project, captureNumber)
	if err != nil {
		if processingErr != nil {
			return fmt.Errorf("%w; capture completion check failed: %v", processingErr, err)
		}
		return err
	}
	if !completed {
		return processingErr
	}

	records, err := p.store.ListCompletedEADRecords(event.Project)
	if err != nil {
		if processingErr != nil {
			return fmt.Errorf("%w; list completed EAD records failed: %v", processingErr, err)
		}
		return err
	}

	reportPath := report.DefaultPath(event.DestinationRoot, event.Project)
	payload := report.Build(event.Project, records)
	if err := report.WriteJSON(reportPath, payload); err != nil {
		if processingErr != nil {
			return fmt.Errorf("%w; write destination report failed: %v", processingErr, err)
		}
		return err
	}

	return processingErr
}

func isEADMetadataFile(path string) bool {
	filename := filepath.Base(path)
	return strings.EqualFold(filepath.Ext(filename), ".xml") && eadMetadataPathRegex.MatchString(filename)
}

func parseCaptureNumber(path string) string {
	filename := filepath.Base(path)
	for _, pattern := range []*regexp.Regexp{eadMetadataPathRegex, rawCapturePathRegex, rawQvPathRegex} {
		matches := pattern.FindStringSubmatch(filename)
		if len(matches) >= 2 {
			return leftPadCaptureNumber(matches[1])
		}
	}
	return ""
}

func joinIssues(issues []Issue) string {
	if len(issues) == 0 {
		return ""
	}

	parts := make([]string, 0, len(issues))
	for _, issue := range issues {
		message := strings.TrimSpace(issue.Message)
		if message == "" {
			continue
		}
		if issue.Line > 0 || issue.Column > 0 {
			parts = append(parts, fmt.Sprintf("%s (line %d, column %d)", message, issue.Line, issue.Column))
			continue
		}
		parts = append(parts, message)
	}

	return strings.Join(parts, "; ")
}
