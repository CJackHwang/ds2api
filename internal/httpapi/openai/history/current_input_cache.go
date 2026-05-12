package history

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"time"

	"ds2api/internal/auth"
)

const (
	currentInputFileCacheTTL        = 5 * time.Minute
	currentInputFileCacheMaxEntries = 256
)

type generatedFileCacheKey struct {
	AccountScope string
	ModelType    string
	Filename     string
	ContentHash  string
}

type generatedFileCacheEntry struct {
	FileID    string
	ExpireAt  time.Time
	CreatedAt time.Time
}

type generatedFileCache struct {
	mu      sync.Mutex
	entries map[generatedFileCacheKey]generatedFileCacheEntry
	now     func() time.Time
}

func newGeneratedFileCache() *generatedFileCache {
	return &generatedFileCache{
		entries: map[generatedFileCacheKey]generatedFileCacheEntry{},
		now:     time.Now,
	}
}

var currentInputToolsFileCache = newGeneratedFileCache()

func currentInputContentHash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func currentInputCacheScope(a *auth.RequestAuth) string {
	if a == nil {
		return ""
	}
	if a.UseConfigToken && strings.TrimSpace(a.AccountID) != "" {
		return "account:" + strings.TrimSpace(a.AccountID)
	}
	if strings.TrimSpace(a.CallerID) != "" {
		return "caller:" + strings.TrimSpace(a.CallerID)
	}
	if strings.TrimSpace(a.DeepSeekToken) == "" {
		return ""
	}
	return "direct-token:" + currentInputContentHash(strings.TrimSpace(a.DeepSeekToken))
}

func (c *generatedFileCache) lookup(key generatedFileCacheKey) (string, bool) {
	if c == nil || key.AccountScope == "" || key.ContentHash == "" {
		return "", false
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.now()
	entry, ok := c.entries[key]
	if !ok {
		return "", false
	}
	if !entry.ExpireAt.After(now) || strings.TrimSpace(entry.FileID) == "" {
		delete(c.entries, key)
		return "", false
	}
	return entry.FileID, true
}

func (c *generatedFileCache) store(key generatedFileCacheKey, fileID string) {
	if c == nil || key.AccountScope == "" || key.ContentHash == "" || strings.TrimSpace(fileID) == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.now()
	c.pruneExpiredLocked(now)
	if len(c.entries) >= currentInputFileCacheMaxEntries {
		c.evictOneLocked()
	}
	c.entries[key] = generatedFileCacheEntry{
		FileID:    strings.TrimSpace(fileID),
		CreatedAt: now,
		ExpireAt:  now.Add(currentInputFileCacheTTL),
	}
}

func (c *generatedFileCache) pruneExpiredLocked(now time.Time) {
	for key, entry := range c.entries {
		if !entry.ExpireAt.After(now) {
			delete(c.entries, key)
		}
	}
}

func (c *generatedFileCache) evictOneLocked() {
	var oldestKey generatedFileCacheKey
	var oldest time.Time
	for key, entry := range c.entries {
		if oldest.IsZero() || entry.CreatedAt.Before(oldest) {
			oldestKey = key
			oldest = entry.CreatedAt
		}
	}
	if !oldest.IsZero() {
		delete(c.entries, oldestKey)
	}
}

// ResetCurrentInputToolsFileCacheForTesting clears the package-level generated
// tools file cache for tests that need deterministic upload counts.
func ResetCurrentInputToolsFileCacheForTesting() {
	currentInputToolsFileCache = newGeneratedFileCache()
}
