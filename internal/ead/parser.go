package ead

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var eadFileNameRegex = regexp.MustCompile(`(?i)^EAD-(\d+)(?:-T)?-.*\.xml$`)

type Record struct {
	CaptureNumber       string
	ProjectName         string
	Area                string
	RecordGUID          string
	SessionID           string
	LineNumber          int
	SegmentNumber       int
	WaypointNumber      int
	ExposureNumber      int
	CapturedAt          time.Time
	Latitude            float64
	Longitude           float64
	Altitude            float64
	AboveGroundLevel    *float64
	TrackOverGround     float64
	GroundSpeed         float64
	Software            string
	Aperture            string
	ExposureTimeSeconds float64
}

type Issue struct {
	Path    string
	Line    int
	Column  int
	Message string
}

type ParseError struct {
	Path   string
	Line   int
	Column int
	Err    error
}

func (e *ParseError) Error() string {
	if e == nil {
		return ""
	}
	if e.Line > 0 || e.Column > 0 {
		return fmt.Sprintf("ead parse error in %s at line %d, column %d: %v", e.Path, e.Line, e.Column, e.Err)
	}
	return fmt.Sprintf("ead parse error in %s: %v", e.Path, e.Err)
}

func (e *ParseError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

type xmlTextValue struct {
	Description string `xml:"description,attr"`
	Value       string `xml:",chardata"`
}

type xmlDocument struct {
	XMLName          xml.Name     `xml:"exposure_annotation_data"`
	ImageNumber      int          `xml:"image_number"`
	RecordGUID       string       `xml:"record_guid"`
	Software         string       `xml:"software"`
	Aperture         xmlTextValue `xml:"aperture"`
	ExposureTime     float64      `xml:"exposure_time"`
	ExposureNumber   int          `xml:"exposure_annotation_info>fms_info>exposure_number"`
	ProjectName      string       `xml:"exposure_annotation_info>fms_info>project_name"`
	Area             string       `xml:"exposure_annotation_info>fms_info>area"`
	LineNumber       int          `xml:"exposure_annotation_info>fms_info>line_number"`
	SegmentNumber    int          `xml:"exposure_annotation_info>fms_info>segment_number"`
	WaypointNumber   int          `xml:"exposure_annotation_info>fms_info>waypoint_number"`
	Date             string       `xml:"exposure_annotation_info>gps_navigation_info>date"`
	Clock            string       `xml:"exposure_annotation_info>gps_navigation_info>time"`
	Latitude         string       `xml:"exposure_annotation_info>gps_navigation_info>latitude"`
	Longitude        string       `xml:"exposure_annotation_info>gps_navigation_info>longitude"`
	Altitude         float64      `xml:"exposure_annotation_info>gps_navigation_info>altitude"`
	TrackOverGround  float64      `xml:"exposure_annotation_info>gps_navigation_info>track_over_ground"`
	AboveGroundLevel *float64     `xml:"exposure_annotation_info>gps_navigation_info>above_ground_level"`
	GroundSpeed      float64      `xml:"exposure_annotation_info>gps_navigation_info>ground_speed"`
}

func ParseFile(path string) (Record, []Issue, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Record{}, nil, err
	}

	return Parse(data, path)
}

func Parse(data []byte, sourcePath string) (Record, []Issue, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))

	var doc xmlDocument
	if err := decoder.Decode(&doc); err != nil {
		line, column := decoder.InputPos()
		return Record{}, nil, &ParseError{Path: sourcePath, Line: line, Column: column, Err: err}
	}

	issues := make([]Issue, 0, 2)

	capturedAt, err := parseTimestamp(doc.Date, doc.Clock)
	if err != nil {
		return Record{}, issues, &ParseError{Path: sourcePath, Err: err}
	}

	latitude, err := parseCoordinate(doc.Latitude)
	if err != nil {
		return Record{}, issues, &ParseError{Path: sourcePath, Err: fmt.Errorf("latitude: %w", err)}
	}
	longitude, err := parseCoordinate(doc.Longitude)
	if err != nil {
		return Record{}, issues, &ParseError{Path: sourcePath, Err: fmt.Errorf("longitude: %w", err)}
	}

	aperture := strings.TrimSpace(doc.Aperture.Description)
	if aperture == "" {
		aperture = strings.TrimSpace(doc.Aperture.Value)
	}

	record := Record{
		CaptureNumber:       normalizeCaptureNumber(sourcePath, doc.ExposureNumber, doc.ImageNumber),
		ProjectName:         strings.TrimSpace(doc.ProjectName),
		Area:                strings.TrimSpace(doc.Area),
		RecordGUID:          strings.ToUpper(strings.TrimSpace(doc.RecordGUID)),
		LineNumber:          doc.LineNumber,
		SegmentNumber:       doc.SegmentNumber,
		WaypointNumber:      doc.WaypointNumber,
		ExposureNumber:      doc.ExposureNumber,
		CapturedAt:          capturedAt,
		Latitude:            latitude,
		Longitude:           longitude,
		Altitude:            doc.Altitude,
		AboveGroundLevel:    doc.AboveGroundLevel,
		TrackOverGround:     doc.TrackOverGround,
		GroundSpeed:         doc.GroundSpeed,
		Software:            strings.TrimSpace(doc.Software),
		Aperture:            aperture,
		ExposureTimeSeconds: doc.ExposureTime,
	}
	record.SessionID = strings.ReplaceAll(record.RecordGUID, "-", "_")

	if doc.ImageNumber > 0 && doc.ExposureNumber > 0 && doc.ImageNumber != doc.ExposureNumber {
		issues = append(issues, Issue{
			Path:    sourcePath,
			Message: fmt.Sprintf("image_number=%d differs from exposure_number=%d; using exposure_number", doc.ImageNumber, doc.ExposureNumber),
		})
	}
	if record.AboveGroundLevel == nil {
		issues = append(issues, Issue{Path: sourcePath, Message: "above_ground_level missing; continuing without AGL"})
	}

	if err := validateRecord(record); err != nil {
		return Record{}, issues, &ParseError{Path: sourcePath, Err: err}
	}

	return record, issues, nil
}

func normalizeCaptureNumber(sourcePath string, numbers ...int) string {
	filename := filepath.Base(sourcePath)
	matches := eadFileNameRegex.FindStringSubmatch(filename)
	if len(matches) == 2 {
		return leftPadCaptureNumber(matches[1])
	}

	for _, number := range numbers {
		if number > 0 {
			return fmt.Sprintf("%05d", number)
		}
	}

	return ""
}

func leftPadCaptureNumber(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if len(raw) >= 5 {
		return raw
	}
	return fmt.Sprintf("%05s", raw)
}

func parseTimestamp(dateValue, timeValue string) (time.Time, error) {
	dateValue = strings.TrimSpace(dateValue)
	timeValue = strings.TrimSpace(timeValue)
	if len(dateValue) != 6 || len(timeValue) != 6 {
		return time.Time{}, fmt.Errorf("invalid date/time combination %q %q", dateValue, timeValue)
	}

	return time.Parse("060102150405", dateValue+timeValue)
}

func parseCoordinate(raw string) (float64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, fmt.Errorf("empty coordinate")
	}

	sign := 1.0
	switch strings.ToUpper(raw[:1]) {
	case "N", "E":
		raw = raw[1:]
	case "S", "W":
		sign = -1
		raw = raw[1:]
	}

	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, err
	}

	return sign * value, nil
}

func validateRecord(record Record) error {
	requiredStrings := map[string]string{
		"capture_number": record.CaptureNumber,
		"project_name":   record.ProjectName,
		"record_guid":    record.RecordGUID,
		"session_id":     record.SessionID,
		"aperture":       record.Aperture,
	}
	for fieldName, value := range requiredStrings {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("missing required field %s", fieldName)
		}
	}

	if record.LineNumber <= 0 || record.SegmentNumber <= 0 || record.WaypointNumber <= 0 || record.ExposureNumber <= 0 {
		return fmt.Errorf("missing required exposure identifiers")
	}
	if record.CapturedAt.IsZero() {
		return fmt.Errorf("missing required timestamp")
	}
	if record.Latitude == 0 && record.Longitude == 0 {
		return fmt.Errorf("missing required coordinates")
	}
	if record.Altitude == 0 {
		return fmt.Errorf("missing required altitude")
	}
	if record.TrackOverGround == 0 {
		return fmt.Errorf("missing required track_over_ground")
	}
	if record.GroundSpeed == 0 {
		return fmt.Errorf("missing required ground_speed")
	}
	if record.ExposureTimeSeconds <= 0 {
		return fmt.Errorf("missing required exposure_time")
	}

	return nil
}
