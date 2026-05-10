package devcapture

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"ds2api/internal/util"

	"github.com/google/uuid"
)

const (
	defaultLimit        = 20
	defaultMaxBodyBytes = 5 * 1024 * 1024
	maxLimit            = 50
)

type Entry struct {
	ID                string `json:"id"`
	CreatedAt         int64  `json:"created_at"`
	Label             string `json:"label"`
	URL               string `json:"url"`
	AccountID         string `json:"account_id,omitempty"`
	StatusCode        int    `json:"status_code"`
	RequestBody       string `json:"request_body"`
	ResponseBody      string `json:"response_body"`
	ResponseTruncated bool   `json:"response_truncated"`
}

type Store struct {
	mu           sync.Mutex
	enabled      bool
	limit        int
	maxBodyBytes int
	items        []Entry
}

type Session struct {
	store      *Store
	id         string
	createdAt  int64
	label      string
	url        string
	accountID  string
	requestRaw string
}

type captureBody struct {
	rc         io.ReadCloser
	s          *Session
	statusCode int
	buf        strings.Builder
	truncated  bool
	finalized  bool
}

var (
	globalOnce sync.Once
	globalInst *Store
)

func Global() *Store {
	globalOnce.Do(func() {
		globalInst = NewFromEnv()
	})
	return globalInst
}

func NewFromEnv() *Store {
	enabled := !isVercelRuntime()
	if raw, ok := os.LookupEnv("DS2API_DEV_PACKET_CAPTURE"); ok {
		enabled = parseBool(raw)
	}
	limit := parseIntWithDefault(os.Getenv("DS2API_DEV_PACKET_CAPTURE_LIMIT"), defaultLimit)
	if limit < 1 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	maxBodyBytes := parseIntWithDefault(os.Getenv("DS2API_DEV_PACKET_CAPTURE_MAX_BODY_BYTES"), defaultMaxBodyBytes)
	if maxBodyBytes < 1024 {
		maxBodyBytes = defaultMaxBodyBytes
	}
	return &Store{
		enabled:      enabled,
		limit:        limit,
		maxBodyBytes: maxBodyBytes,
		items:        make([]Entry, 0, limit),
	}
}

func isVercelRuntime() bool {
	return strings.TrimSpace(os.Getenv("VERCEL")) != "" || strings.TrimSpace(os.Getenv("NOW_REGION")) != ""
}

func (s *Store) Enabled() bool {
	if s == nil {
		return false
	}
	return s.enabled
}

func (s *Store) Limit() int {
	if s == nil {
		return defaultLimit
	}
	return s.limit
}

func (s *Store) MaxBodyBytes() int {
	if s == nil {
		return defaultMaxBodyBytes
	}
	return s.maxBodyBytes
}

func (s *Store) Snapshot() []Entry {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Entry, len(s.items))
	copy(out, s.items)
	return out
}

func (s *Store) Clear() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = s.items[:0]
}

func (s *Store) Start(label, url, accountID string, requestPayload any) *Session {
	if s == nil || !s.enabled {
		return nil
	}
	return &Session{
		store:      s,
		id:         "cap_" + strings.ReplaceAll(uuid.NewString(), "-", ""),
		createdAt:  time.Now().Unix(),
		label:      strings.TrimSpace(label),
		url:        strings.TrimSpace(url),
		accountID:  strings.TrimSpace(accountID),
		requestRaw: util.RedactSensitiveFields(marshalPayload(requestPayload)),
	}
}

func (s *Session) WrapBody(rc io.ReadCloser, statusCode int) io.ReadCloser {
	if s == nil || rc == nil {
		return rc
	}
	return &captureBody{
		rc:         rc,
		s:          s,
		statusCode: statusCode,
	}
}

func (c *captureBody) Read(p []byte) (int, error) {
	n, err := c.rc.Read(p)
	if n > 0 {
		c.append(string(p[:n]))
	}
	if err == io.EOF {
		c.finalize()
	}
	return n, err
}

func (c *captureBody) Close() error {
	err := c.rc.Close()
	c.finalize()
	return err
}

func (c *captureBody) append(chunk string) {
	if chunk == "" || c.s == nil || c.s.store == nil {
		return
	}
	maxLen := c.s.store.maxBodyBytes
	current := c.buf.Len()
	if current >= maxLen {
		c.truncated = true
		return
	}
	remain := maxLen - current
	if len(chunk) > remain {
		truncated, _ := util.TruncateUTF8Bytes(chunk, remain)
		c.buf.WriteString(truncated)
		c.truncated = true
		return
	}
	c.buf.WriteString(chunk)
}

func (c *captureBody) finalize() {
	if c.finalized || c.s == nil || c.s.store == nil {
		return
	}
	c.finalized = true
	entry := Entry{
		ID:                c.s.id,
		CreatedAt:         c.s.createdAt,
		Label:             c.s.label,
		URL:               c.s.url,
		AccountID:         c.s.accountID,
		StatusCode:        c.statusCode,
		RequestBody:       c.s.requestRaw,
		ResponseBody:      c.buf.String(),
		ResponseTruncated: c.truncated,
	}
	c.s.store.push(entry)
}

func (s *Store) push(entry Entry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items = append([]Entry{entry}, s.items...)
	if len(s.items) > s.limit {
		s.items = s.items[:s.limit]
	}
}

func marshalPayload(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

func parseBool(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func parseIntWithDefault(raw string, d int) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return d
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return d
	}
	return n
}
