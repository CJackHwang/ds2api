package history

import (
	"strings"
	"sync"
	"time"

	"ds2api/internal/config"
	"ds2api/internal/promptcompat"
)

// SessionState holds the DeepSeek session state for a conversation.
type SessionState struct {
	ChatSessionID   string
	ParentMessageID int
	MessagesCount   int
	CreatedAt       time.Time
}

// SessionStore manages session state for conversations.
// For the experiment, each account has at most one active session.
type SessionStore struct {
	mu      sync.RWMutex
	entries map[string]*SessionState // keyed by accountID
	ttl     time.Duration
}

// NewSessionStore creates a new session store with the given TTL.
func NewSessionStore(ttl time.Duration) *SessionStore {
	return &SessionStore{
		entries: make(map[string]*SessionState),
		ttl:     ttl,
	}
}

// Get retrieves the session state for the given account.
func (s *SessionStore) Get(accountID string) (*SessionState, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	state, ok := s.entries[accountID]
	if !ok {
		return nil, false
	}
	if time.Since(state.CreatedAt) > s.ttl {
		return nil, false
	}
	return state, true
}

// Set stores the session state for the given account.
func (s *SessionStore) Set(accountID string, state *SessionState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[accountID] = state
}

// ApplySessionContext checks if we have an existing session for this account.
// If yes, it modifies the request to only send the last user message and sets session state.
// If no, it returns the request unchanged (first request or new conversation).
func (s *SessionStore) ApplySessionContext(stdReq promptcompat.StandardRequest, accountID string) promptcompat.StandardRequest {
	if s == nil || accountID == "" {
		return stdReq
	}

	// Extract the last user message.
	lastIdx, lastText := latestUserInputForFile(stdReq.Messages)
	if lastIdx < 0 || strings.TrimSpace(lastText) == "" {
		return stdReq
	}

	// Look up existing session for this account.
	state, ok := s.Get(accountID)
	if !ok {
		// No session yet — first request of conversation.
		return stdReq
	}

	// Check if the messages prefix matches what DeepSeek has seen.
	// DeepSeek has seen state.MessagesCount messages. If the current request
	// has more messages and the prefix matches, this is an incremental request.
	if len(stdReq.Messages) <= state.MessagesCount {
		// Fewer or same messages — might be a new conversation or reset.
		return stdReq
	}

	// We have an existing session with more messages — send only the last message.
	stdReq.SessionChatID = state.ChatSessionID
	stdReq.SessionParentMsgID = state.ParentMessageID
	stdReq.SessionIncremental = true

	// Replace messages with just the last user message.
	stdReq.Messages = []any{
		map[string]any{
			"role":    "user",
			"content": lastText,
		},
	}

	// Rebuild prompt from just the last message.
	finalPrompt, toolNames := promptcompat.BuildOpenAIPrompt(stdReq.Messages, stdReq.ToolsRaw, "", stdReq.ToolChoice, stdReq.Thinking)
	stdReq.FinalPrompt = finalPrompt
	stdReq.ToolNames = toolNames
	stdReq.PromptTokenText = finalPrompt

	config.Logger.Debug("[session_context] using persistent session",
		"chat_session_id", state.ChatSessionID,
		"parent_message_id", state.ParentMessageID,
		"messages_count", len(stdReq.Messages),
		"previous_count", state.MessagesCount,
	)

	return stdReq
}

// StoreResponse stores the response state for a conversation.
// messagesCount is the number of messages that were sent to DeepSeek (before this response).
func (s *SessionStore) StoreResponse(accountID string, messagesCount int, chatSessionID string, responseMessageID int) {
	if s == nil || accountID == "" || chatSessionID == "" || responseMessageID <= 0 {
		return
	}
	s.Set(accountID, &SessionState{
		ChatSessionID:   chatSessionID,
		ParentMessageID: responseMessageID,
		MessagesCount:   messagesCount,
		CreatedAt:       time.Now(),
	})
	config.Logger.Debug("[session_context] stored response state",
		"account_id", accountID,
		"chat_session_id", chatSessionID,
		"response_message_id", responseMessageID,
		"messages_count", messagesCount,
	)
}
