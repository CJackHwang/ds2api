package chat

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"ds2api/internal/config"
)

type SessionCacheEntry struct {
	SessionID       string      `json:"session_id"`
	ParentMessageID interface{} `json:"parent_message_id"`
	LastMessageID   interface{} `json:"last_message_id"`
	HistoryText     string      `json:"history_text"`
	CreatedAt       time.Time   `json:"created_at"`
	LastUsedAt      time.Time   `json:"last_used_at"`
	MessageCount    int         `json:"message_count"`
}

type SessionCache struct {
	mu       sync.RWMutex
	entries  map[string]*SessionCacheEntry
	filePath string
}

var globalSessionCache *SessionCache

func InitSessionCache() {
	cacheDir := filepath.Join(".", "data", "sessions")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		config.Logger.Warn("[session_cache] failed to create session directory", "path", cacheDir, "error", err)
	}
	filePath := filepath.Join(cacheDir, "session_cache.json")

	globalSessionCache = &SessionCache{
		entries:  make(map[string]*SessionCacheEntry),
		filePath: filePath,
	}
	globalSessionCache.loadFromFile()
	config.Logger.Info("[session_cache] initialized with persistence", "path", filePath)
}

func GetSessionCache() *SessionCache {
	if globalSessionCache == nil {
		InitSessionCache()
	}
	return globalSessionCache
}

func (c *SessionCache) loadFromFile() {
	c.mu.Lock()
	defer c.mu.Unlock()

	data, err := os.ReadFile(c.filePath)
	if err != nil {
		if !os.IsNotExist(err) {
			config.Logger.Warn("[session_cache] failed to load from file", "error", err)
		}
		return
	}

	var entries map[string]*SessionCacheEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		config.Logger.Warn("[session_cache] failed to parse cache file", "error", err)
		return
	}

	c.entries = entries
	config.Logger.Info("[session_cache] loaded from file", "entries", len(c.entries))
}

func (c *SessionCache) saveToFile() {
	data, err := json.MarshalIndent(c.entries, "", "  ")
	if err != nil {
		config.Logger.Warn("[session_cache] failed to marshal cache", "error", err)
		return
	}
	if err := os.WriteFile(c.filePath, data, 0644); err != nil {
		config.Logger.Warn("[session_cache] failed to save to file", "error", err)
	}
}

func (c *SessionCache) GetSession(token string) (string, interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.entries[token]
	if !ok || entry.SessionID == "" {
		return "", nil, false
	}
	return entry.SessionID, entry.ParentMessageID, true
}

func (c *SessionCache) SetSession(token, sessionID string, parentMessageID interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, exists := c.entries[token]
	if exists {
		entry.SessionID = sessionID
		entry.ParentMessageID = parentMessageID
		entry.LastUsedAt = time.Now()
	} else {
		c.entries[token] = &SessionCacheEntry{
			SessionID:       sessionID,
			ParentMessageID: parentMessageID,
			CreatedAt:       time.Now(),
			LastUsedAt:      time.Now(),
		}
	}
	c.saveToFile()
}

func (c *SessionCache) UpdateMessageIDs(token string, lastMessageID, newParentMessageID interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[token]
	if !ok {
		return
	}
	entry.LastMessageID = lastMessageID
	entry.ParentMessageID = newParentMessageID
	entry.MessageCount++
	entry.LastUsedAt = time.Now()
	c.saveToFile()
}

func (c *SessionCache) GetHistoryText(token string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.entries[token]
	if !ok {
		return ""
	}
	return entry.HistoryText
}

func (c *SessionCache) SetHistoryText(token, historyText string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[token]
	if !ok {
		return
	}
	entry.HistoryText = historyText
	c.saveToFile()
}

func (c *SessionCache) GetMessageCount(token string) int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.entries[token]
	if !ok {
		return 0
	}
	return entry.MessageCount
}

func (c *SessionCache) IsFirstMessage(token string) bool {
	return c.GetMessageCount(token) == 0
}

func (c *SessionCache) Remove(token string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, token)
	c.saveToFile()
}

func (c *SessionCache) Cleanup(maxAge time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	for token, entry := range c.entries {
		if now.Sub(entry.LastUsedAt) > maxAge {
			delete(c.entries, token)
			config.Logger.Info("[session_cache] cleaned up expired entry", "token", token[:min(len(token), 8)]+"...")
		}
	}
}

func KeepSessionEnabled() bool {
	return strings.TrimSpace(os.Getenv("KEEP_SESSION")) == "true"
}
