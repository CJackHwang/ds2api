package historyanalyzer

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"ds2api/internal/devcapture"
	"ds2api/internal/rawsample"
)

type DevCaptureLoadOptions struct {
	Path     string
	Redactor Redactor
}

type RawSampleLoadOptions struct {
	Path     string
	Redactor Redactor
}

func LoadDevCaptures(opts DevCaptureLoadOptions) ([]AnalysisRecord, ReportScope, error) {
	path := strings.TrimSpace(opts.Path)
	if path == "" {
		return nil, ReportScope{}, errors.New("dev capture path is required")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, ReportScope{}, fmt.Errorf("read dev capture: %w", err)
	}
	entries, err := decodeDevCaptureEntries(raw)
	if err != nil {
		return nil, ReportScope{}, err
	}
	source := SourceRef{Kind: "devcapture", Path: path}
	scope := ReportScope{
		Name:    "dev capture",
		Sources: []SourceRef{source},
	}
	return RecordsFromDevCapture(entries, opts.Redactor), scope, nil
}

func LoadRawSamples(opts RawSampleLoadOptions) ([]AnalysisRecord, ReportScope, error) {
	path := strings.TrimSpace(opts.Path)
	if path == "" {
		return nil, ReportScope{}, errors.New("raw sample path is required")
	}
	metaPaths, err := rawSampleMetaPaths(path)
	if err != nil {
		return nil, ReportScope{}, err
	}
	records := make([]AnalysisRecord, 0, len(metaPaths))
	for _, metaPath := range metaPaths {
		meta, err := readRawSampleMeta(metaPath)
		if err != nil {
			return nil, ReportScope{}, err
		}
		record, err := recordFromRawSampleDir(meta, filepath.Dir(metaPath), opts.Redactor)
		if err != nil {
			return nil, ReportScope{}, err
		}
		records = append(records, record)
	}
	source := SourceRef{Kind: "rawsample", Path: path}
	scope := ReportScope{
		Name:    "raw samples",
		Sources: []SourceRef{source},
	}
	return records, scope, nil
}

func decodeDevCaptureEntries(raw []byte) ([]devcapture.Entry, error) {
	var entries []devcapture.Entry
	if err := json.Unmarshal(raw, &entries); err == nil {
		return entries, nil
	}

	var envelope struct {
		Items []devcapture.Entry `json:"items"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("decode dev capture: %w", err)
	}
	return envelope.Items, nil
}

func rawSampleMetaPaths(path string) ([]string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat raw sample path: %w", err)
	}
	if !info.IsDir() {
		return []string{path}, nil
	}

	directMeta := filepath.Join(path, "meta.json")
	if stat, err := os.Stat(directMeta); err == nil && !stat.IsDir() {
		return []string{directMeta}, nil
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("read raw sample dir: %w", err)
	}
	metaPaths := make([]string, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		candidate := filepath.Join(path, entry.Name(), "meta.json")
		if stat, err := os.Stat(candidate); err == nil && !stat.IsDir() {
			metaPaths = append(metaPaths, candidate)
		}
	}
	sort.Strings(metaPaths)
	if len(metaPaths) == 0 {
		return nil, fmt.Errorf("raw sample dir %q contains no meta.json files", path)
	}
	return metaPaths, nil
}

func readRawSampleMeta(path string) (rawsample.Meta, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return rawsample.Meta{}, fmt.Errorf("read raw sample meta: %w", err)
	}
	var meta rawsample.Meta
	if err := json.Unmarshal(raw, &meta); err != nil {
		return rawsample.Meta{}, fmt.Errorf("decode raw sample meta: %w", err)
	}
	return meta, nil
}

func recordFromRawSampleDir(meta rawsample.Meta, dir string, redactor Redactor) (AnalysisRecord, error) {
	record := RecordFromRawSample(meta, dir, redactor)
	streamText, err := readOptionalRawSampleStream(dir)
	if err != nil {
		return AnalysisRecord{}, err
	}
	if strings.TrimSpace(streamText) == "" {
		return record, nil
	}
	if record.Text == nil {
		record.Text = map[string]string{}
	}
	addText(record.Text, "upstream_text", streamText)
	record.Snapshots = snapshotsFromText(record.Text, redactor)
	return record, nil
}

func readOptionalRawSampleStream(dir string) (string, error) {
	path := filepath.Join(dir, "upstream.stream.sse")
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("read raw sample upstream stream: %w", err)
	}
	return string(raw), nil
}
