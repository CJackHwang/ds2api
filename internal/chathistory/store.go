package chathistory

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"ds2api/internal/config"
)

const (
	FileVersion      = 2
	DisabledLimit    = 0
	DefaultLimit     = 20
	MaxLimit         = 50
	defaultPreviewAt = 160
)

var allowedLimits = map[int]struct{}{
	DisabledLimit: {},
	10:            {},
	20:            {},
	50:            {},
}

var ErrDisabled = errors.New("chat history disabled")

type Entry struct {
	ID               string         `json:"id"`
	Revision         int64          `json:"revision"`
	CreatedAt        int64          `json:"created_at"`
	UpdatedAt        int64          `json:"updated_at"`
	CompletedAt      int64          `json:"completed_at,omitempty"`
	Status           string         `json:"status"`
	CallerID         string         `json:"caller_id,omitempty"`
	AccountID        string         `json:"account_id,omitempty"`
	Model            string         `json:"model,omitempty"`
	Stream           bool           `json:"stream"`
	UserInput        string         `json:"user_input,omitempty"`
	Messages         []Message      `json:"messages,omitempty"`
	HistoryText      string         `json:"history_text,omitempty"`
	FinalPrompt      string         `json:"final_prompt,omitempty"`
	Diagnostics      *Diagnostics   `json:"diagnostics,omitempty"`
	ReasoningContent string         `json:"reasoning_content,omitempty"`
	Content          string         `json:"content,omitempty"`
	Error            string         `json:"error,omitempty"`
	StatusCode       int            `json:"status_code,omitempty"`
	ElapsedMs        int64          `json:"elapsed_ms,omitempty"`
	FinishReason     string         `json:"finish_reason,omitempty"`
	Usage            map[string]any `json:"usage,omitempty"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type SummaryEntry struct {
	ID             string `json:"id"`
	Revision       int64  `json:"revision"`
	CreatedAt      int64  `json:"created_at"`
	UpdatedAt      int64  `json:"updated_at"`
	CompletedAt    int64  `json:"completed_at,omitempty"`
	Status         string `json:"status"`
	CallerID       string `json:"caller_id,omitempty"`
	AccountID      string `json:"account_id,omitempty"`
	Model          string `json:"model,omitempty"`
	Stream         bool   `json:"stream"`
	UserInput      string `json:"user_input,omitempty"`
	Preview        string `json:"preview,omitempty"`
	StatusCode     int    `json:"status_code,omitempty"`
	ElapsedMs      int64  `json:"elapsed_ms,omitempty"`
	FinishReason   string `json:"finish_reason,omitempty"`
	DetailRevision int64  `json:"detail_revision"`
}

type File struct {
	Version  int            `json:"version"`
	Limit    int            `json:"limit"`
	Revision int64          `json:"revision"`
	Items    []SummaryEntry `json:"items"`
}

type StartParams struct {
	CallerID    string
	AccountID   string
	Model       string
	Stream      bool
	UserInput   string
	Messages    []Message
	HistoryText string
	FinalPrompt string
	Diagnostics *Diagnostics
}

type Diagnostics struct {
	CurrentInputUpload *FileUpload `json:"current_input_upload,omitempty"`
	HistoryUpload      *FileUpload `json:"history_upload,omitempty"`
	CurrentInputReason string      `json:"current_input_reason,omitempty"`
	HistoryReason      string      `json:"history_reason,omitempty"`
	RefFileIDs         []string    `json:"ref_file_ids,omitempty"`
}

type FileUpload struct {
	Filename string `json:"filename,omitempty"`
	Bytes    int    `json:"bytes,omitempty"`
	FileID   string `json:"file_id,omitempty"`
}

type UpdateParams struct {
	Status           string
	ReasoningContent string
	Content          string
	Error            string
	StatusCode       int
	ElapsedMs        int64
	FinishReason     string
	Usage            map[string]any
	Completed        bool
}

type detailEnvelope struct {
	Version int   `json:"version"`
	Item    Entry `json:"item"`
}

type legacyFile struct {
	Version int     `json:"version"`
	Limit   int     `json:"limit"`
	Items   []Entry `json:"items"`
}

type legacyProbe struct {
	Items []map[string]json.RawMessage `json:"items"`
}

type Store struct {
	mu        sync.Mutex
	path      string
	detailDir string
	state     File
	details   map[string]Entry
	dirty     map[string]struct{}
	deleted   map[string]struct{}
	err       error
}

func New(path string) *Store {
	s := &Store{
		path:      strings.TrimSpace(path),
		detailDir: strings.TrimSpace(path) + ".d",
		state: File{
			Version:  FileVersion,
			Limit:    DefaultLimit,
			Revision: 0,
			Items:    []SummaryEntry{},
		},
		details: map[string]Entry{},
		dirty:   map[string]struct{}{},
		deleted: map[string]struct{}{},
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.err = s.loadLocked()
	return s
}

func (s *Store) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

func (s *Store) DetailDir() string {
	if s == nil {
		return ""
	}
	return s.detailDir
}

func (s *Store) Err() error {
	if s == nil {
		return errors.New("chat history store is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

func (s *Store) Snapshot() (File, error) {
	if s == nil {
		return File{}, errors.New("chat history store is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return File{}, s.err
	}
	return cloneFile(s.state), nil
}

func (s *Store) Enabled() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return false
	}
	return s.state.Limit != DisabledLimit
}

func (s *Store) Get(id string) (Entry, error) {
	if s == nil {
		return Entry{}, errors.New("chat history store is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return Entry{}, s.err
	}
	item, ok := s.details[strings.TrimSpace(id)]
	if !ok {
		return Entry{}, errors.New("chat history entry not found")
	}
	return cloneEntry(item), nil
}

func (s *Store) Start(params StartParams) (Entry, error) {
	if s == nil {
		return Entry{}, errors.New("chat history store is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return Entry{}, s.err
	}
	if s.state.Limit == DisabledLimit {
		return Entry{}, ErrDisabled
	}
	now := time.Now().UnixMilli()
	revision := s.nextRevisionLocked()
	entry := Entry{
		ID:          "chat_" + strings.ReplaceAll(uuid.NewString(), "-", ""),
		Revision:    revision,
		CreatedAt:   now,
		UpdatedAt:   now,
		Status:      "streaming",
		CallerID:    strings.TrimSpace(params.CallerID),
		AccountID:   strings.TrimSpace(params.AccountID),
		Model:       strings.TrimSpace(params.Model),
		Stream:      params.Stream,
		UserInput:   strings.TrimSpace(params.UserInput),
		Messages:    cloneMessages(params.Messages),
		HistoryText: params.HistoryText,
		FinalPrompt: strings.TrimSpace(params.FinalPrompt),
		Diagnostics: cloneDiagnostics(params.Diagnostics),
	}
	s.details[entry.ID] = entry
	s.markDetailDirtyLocked(entry.ID)
	s.rebuildIndexLocked()
	if err := s.saveLocked(); err != nil {
		return cloneEntry(entry), err
	}
	return cloneEntry(entry), nil
}

func (s *Store) Update(id string, params UpdateParams) (Entry, error) {
	if s == nil {
		return Entry{}, errors.New("chat history store is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return Entry{}, s.err
	}
	target := strings.TrimSpace(id)
	if target == "" {
		return Entry{}, errors.New("history id is required")
	}
	item, ok := s.details[target]
	if !ok {
		return Entry{}, errors.New("chat history entry not found")
	}
	now := time.Now().UnixMilli()
	item.Revision = s.nextRevisionLocked()
	item.UpdatedAt = now
	if params.Status != "" {
		item.Status = params.Status
	}
	item.ReasoningContent = params.ReasoningContent
	item.Content = params.Content
	item.Error = strings.TrimSpace(params.Error)
	item.StatusCode = params.StatusCode
	item.ElapsedMs = params.ElapsedMs
	item.FinishReason = strings.TrimSpace(params.FinishReason)
	if params.Usage != nil {
		item.Usage = cloneMap(params.Usage)
	}
	if params.Completed {
		item.CompletedAt = now
	}
	s.details[target] = item
	s.markDetailDirtyLocked(target)
	s.rebuildIndexLocked()
	if err := s.saveLocked(); err != nil {
		return Entry{}, err
	}
	return cloneEntry(item), nil
}

func (s *Store) Delete(id string) error {
	if s == nil {
		return errors.New("chat history store is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	target := strings.TrimSpace(id)
	if target == "" {
		return errors.New("history id is required")
	}
	if _, ok := s.details[target]; !ok {
		return errors.New("chat history entry not found")
	}
	s.markDetailDeletedLocked(target)
	delete(s.details, target)
	s.nextRevisionLocked()
	s.rebuildIndexLocked()
	if err := s.saveLocked(); err != nil {
		return err
	}
	return nil
}

func (s *Store) Clear() error {
	if s == nil {
		return errors.New("chat history store is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	for id := range s.details {
		s.markDetailDeletedLocked(id)
	}
	s.details = map[string]Entry{}
	s.nextRevisionLocked()
	s.rebuildIndexLocked()
	if err := s.saveLocked(); err != nil {
		return err
	}
	return nil
}

func (s *Store) SetLimit(limit int) (File, error) {
	if s == nil {
		return File{}, errors.New("chat history store is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return File{}, s.err
	}
	if !isAllowedLimit(limit) {
		return File{}, fmt.Errorf("unsupported chat history limit: %d", limit)
	}
	s.state.Limit = limit
	s.nextRevisionLocked()
	s.rebuildIndexLocked()
	if err := s.saveLocked(); err != nil {
		return File{}, err
	}
	return cloneFile(s.state), nil
}

func (s *Store) loadLocked() error {
	if strings.TrimSpace(s.path) == "" {
		return errors.New("chat history path is required")
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil && filepath.Dir(s.path) != "." {
		return fmt.Errorf("create chat history dir: %w", err)
	}
	if err := os.MkdirAll(s.detailDir, 0o755); err != nil {
		return fmt.Errorf("create chat history detail dir: %w", err)
	}

	raw, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if saveErr := s.saveLocked(); saveErr != nil {
				config.Logger.Warn("[chat_history] bootstrap write failed", "path", s.path, "error", saveErr)
			}
			return nil
		}
		return fmt.Errorf("read chat history index: %w", err)
	}

	legacy, legacyOK, legacyErr := parseLegacy(raw)
	if legacyErr != nil {
		return legacyErr
	}
	if legacyOK {
		s.loadLegacyLocked(legacy)
		if err := s.saveLocked(); err != nil {
			config.Logger.Warn("[chat_history] legacy migration writeback failed", "path", s.path, "error", err)
		}
		return nil
	}

	var state File
	if err := json.Unmarshal(raw, &state); err != nil {
		return fmt.Errorf("decode chat history index: %w", err)
	}
	if state.Version == 0 {
		state.Version = FileVersion
	}
	if !isAllowedLimit(state.Limit) {
		state.Limit = DefaultLimit
	}
	s.state = cloneFile(state)
	s.details = map[string]Entry{}
	for _, item := range state.Items {
		detail, err := readDetailFile(filepath.Join(s.detailDir, item.ID+".json"))
		if err != nil {
			return err
		}
		s.details[item.ID] = detail
	}
	s.rebuildIndexLocked()
	if saveErr := s.saveLocked(); saveErr != nil {
		config.Logger.Warn("[chat_history] index rewrite failed", "path", s.path, "error", saveErr)
	}
	return nil
}

func (s *Store) loadLegacyLocked(legacy legacyFile) {
	s.state.Version = FileVersion
	s.state.Limit = legacy.Limit
	if !isAllowedLimit(s.state.Limit) {
		s.state.Limit = DefaultLimit
	}
	s.details = map[string]Entry{}
	s.dirty = map[string]struct{}{}
	s.deleted = map[string]struct{}{}
	maxRevision := int64(0)
	for _, item := range legacy.Items {
		if strings.TrimSpace(item.ID) == "" {
			continue
		}
		item.Messages = cloneMessages(item.Messages)
		if item.Revision == 0 {
			if item.UpdatedAt > 0 {
				item.Revision = item.UpdatedAt
			} else {
				item.Revision = time.Now().UnixNano()
			}
		}
		if item.Revision > maxRevision {
			maxRevision = item.Revision
		}
		s.details[item.ID] = item
		s.markDetailDirtyLocked(item.ID)
	}
	s.state.Revision = maxRevision
	s.rebuildIndexLocked()
}

func (s *Store) saveLocked() error {
	s.state.Version = FileVersion
	if !isAllowedLimit(s.state.Limit) {
		s.state.Limit = DefaultLimit
	}
	s.rebuildIndexLocked()

	if err := os.MkdirAll(s.detailDir, 0o755); err != nil {
		return fmt.Errorf("create chat history detail dir: %w", err)
	}
	for _, id := range sortedDetailIDs(s.deleted) {
		path := filepath.Join(s.detailDir, id+".json")
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove stale chat history detail: %w", err)
		}
	}
	for _, id := range sortedDetailIDs(s.dirty) {
		item, ok := s.details[id]
		if !ok {
			continue
		}
		path := filepath.Join(s.detailDir, id+".json")
		payload, err := json.MarshalIndent(detailEnvelope{
			Version: FileVersion,
			Item:    item,
		}, "", "  ")
		if err != nil {
			return fmt.Errorf("encode chat history detail: %w", err)
		}
		if err := writeFileAtomic(path, append(payload, '\n')); err != nil {
			return err
		}
	}

	payload, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode chat history index: %w", err)
	}
	if err := writeFileAtomic(s.path, append(payload, '\n')); err != nil {
		return err
	}
	s.clearPendingDetailChangesLocked()
	return nil
}

func (s *Store) rebuildIndexLocked() {
	summaries := make([]SummaryEntry, 0, len(s.details))
	for _, item := range s.details {
		summaries = append(summaries, summaryFromEntry(item))
	}
	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].UpdatedAt == summaries[j].UpdatedAt {
			return summaries[i].CreatedAt > summaries[j].CreatedAt
		}
		return summaries[i].UpdatedAt > summaries[j].UpdatedAt
	})
	if s.state.Limit < DisabledLimit || !isAllowedLimit(s.state.Limit) {
		s.state.Limit = DefaultLimit
	}
	if s.state.Limit == DisabledLimit {
		s.state.Items = summaries
		return
	}
	if len(summaries) > s.state.Limit {
		keep := make(map[string]struct{}, s.state.Limit)
		for _, item := range summaries[:s.state.Limit] {
			keep[item.ID] = struct{}{}
		}
		for id := range s.details {
			if _, ok := keep[id]; !ok {
				s.markDetailDeletedLocked(id)
				delete(s.details, id)
			}
		}
		summaries = summaries[:s.state.Limit]
	}
	s.state.Items = summaries
}

func (s *Store) nextRevisionLocked() int64 {
	next := time.Now().UnixNano()
	if next <= s.state.Revision {
		next = s.state.Revision + 1
	}
	s.state.Revision = next
	return next
}

func summaryFromEntry(item Entry) SummaryEntry {
	return SummaryEntry{
		ID:             item.ID,
		Revision:       item.Revision,
		CreatedAt:      item.CreatedAt,
		UpdatedAt:      item.UpdatedAt,
		CompletedAt:    item.CompletedAt,
		Status:         item.Status,
		CallerID:       item.CallerID,
		AccountID:      item.AccountID,
		Model:          item.Model,
		Stream:         item.Stream,
		UserInput:      item.UserInput,
		Preview:        buildPreview(item),
		StatusCode:     item.StatusCode,
		ElapsedMs:      item.ElapsedMs,
		FinishReason:   item.FinishReason,
		DetailRevision: item.Revision,
	}
}

func buildPreview(item Entry) string {
	candidate := strings.TrimSpace(item.Content)
	if candidate == "" {
		candidate = strings.TrimSpace(item.ReasoningContent)
	}
	if candidate == "" {
		candidate = strings.TrimSpace(item.Error)
	}
	if candidate == "" {
		candidate = strings.TrimSpace(item.UserInput)
	}
	if len(candidate) > defaultPreviewAt {
		return candidate[:defaultPreviewAt] + "..."
	}
	return candidate
}

func readDetailFile(path string) (Entry, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Entry{}, fmt.Errorf("read chat history detail: %w", err)
	}
	var env detailEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return Entry{}, fmt.Errorf("decode chat history detail: %w", err)
	}
	return cloneEntry(env.Item), nil
}

func parseLegacy(raw []byte) (legacyFile, bool, error) {
	var legacy legacyFile
	if err := json.Unmarshal(raw, &legacy); err != nil {
		return legacyFile{}, false, nil
	}
	if len(legacy.Items) == 0 {
		return legacy, false, nil
	}
	var probe legacyProbe
	if err := json.Unmarshal(raw, &probe); err == nil {
		for _, item := range probe.Items {
			if _, ok := item["detail_revision"]; ok {
				return legacy, false, nil
			}
		}
	}
	return legacy, true, nil
}

func writeFileAtomic(path string, body []byte) error {
	dir := filepath.Dir(path)
	if dir == "" {
		dir = "."
	}
	if dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create chat history dir: %w", err)
		}
	}
	tmpFile, err := os.CreateTemp(dir, ".chat-history-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp chat history: %w", err)
	}
	tmpPath := tmpFile.Name()
	cleanup := func() error {
		if err := os.Remove(tmpPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove temp chat history: %w", err)
		}
		return nil
	}
	withCleanup := func(primary error, closeErr error) error {
		errs := []error{primary}
		if closeErr != nil {
			errs = append(errs, fmt.Errorf("close temp chat history: %w", closeErr))
		}
		if cleanupErr := cleanup(); cleanupErr != nil {
			errs = append(errs, cleanupErr)
		}
		return errors.Join(errs...)
	}
	if _, err := tmpFile.Write(body); err != nil {
		return withCleanup(fmt.Errorf("write temp chat history: %w", err), tmpFile.Close())
	}
	if err := tmpFile.Sync(); err != nil {
		return withCleanup(fmt.Errorf("sync temp chat history: %w", err), tmpFile.Close())
	}
	if err := tmpFile.Close(); err != nil {
		if cleanupErr := cleanup(); cleanupErr != nil {
			return errors.Join(fmt.Errorf("close temp chat history: %w", err), cleanupErr)
		}
		return fmt.Errorf("close temp chat history: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		if cleanupErr := cleanup(); cleanupErr != nil {
			return errors.Join(fmt.Errorf("promote temp chat history: %w", err), cleanupErr)
		}
		return fmt.Errorf("promote temp chat history: %w", err)
	}
	return nil
}

func ListETag(revision int64) string {
	return fmt.Sprintf(`W/"chat-history-list-%d"`, revision)
}

func DetailETag(id string, revision int64) string {
	return fmt.Sprintf(`W/"chat-history-detail-%s-%d"`, strings.TrimSpace(id), revision)
}

func isAllowedLimit(limit int) bool {
	_, ok := allowedLimits[limit]
	return ok
}

func (s *Store) markDetailDirtyLocked(id string) {
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	if s.dirty == nil {
		s.dirty = map[string]struct{}{}
	}
	if s.deleted == nil {
		s.deleted = map[string]struct{}{}
	}
	s.dirty[id] = struct{}{}
	delete(s.deleted, id)
}

func (s *Store) markDetailDeletedLocked(id string) {
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	if s.dirty == nil {
		s.dirty = map[string]struct{}{}
	}
	if s.deleted == nil {
		s.deleted = map[string]struct{}{}
	}
	s.deleted[id] = struct{}{}
	delete(s.dirty, id)
}

func (s *Store) clearPendingDetailChangesLocked() {
	s.dirty = map[string]struct{}{}
	s.deleted = map[string]struct{}{}
}

func sortedDetailIDs(ids map[string]struct{}) []string {
	if len(ids) == 0 {
		return nil
	}
	out := make([]string, 0, len(ids))
	for id := range ids {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func cloneFile(in File) File {
	out := File{
		Version:  in.Version,
		Limit:    in.Limit,
		Revision: in.Revision,
		Items:    make([]SummaryEntry, len(in.Items)),
	}
	copy(out.Items, in.Items)
	return out
}

func cloneEntry(item Entry) Entry {
	item.Usage = cloneMap(item.Usage)
	item.Messages = cloneMessages(item.Messages)
	item.Diagnostics = cloneDiagnostics(item.Diagnostics)
	return item
}

func cloneMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneMessages(messages []Message) []Message {
	if len(messages) == 0 {
		return []Message{}
	}
	out := make([]Message, len(messages))
	copy(out, messages)
	return out
}

func cloneDiagnostics(in *Diagnostics) *Diagnostics {
	if in == nil {
		return nil
	}
	out := &Diagnostics{
		CurrentInputUpload: cloneFileUpload(in.CurrentInputUpload),
		HistoryUpload:      cloneFileUpload(in.HistoryUpload),
		CurrentInputReason: strings.TrimSpace(in.CurrentInputReason),
		HistoryReason:      strings.TrimSpace(in.HistoryReason),
		RefFileIDs:         cloneStringSlice(in.RefFileIDs),
	}
	if out.CurrentInputUpload == nil && out.HistoryUpload == nil && out.CurrentInputReason == "" && out.HistoryReason == "" && len(out.RefFileIDs) == 0 {
		return nil
	}
	return out
}

func cloneFileUpload(in *FileUpload) *FileUpload {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func cloneStringSlice(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}
