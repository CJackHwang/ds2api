package openai

import (
	"context"
	"net/http"

	"ds2api/internal/auth"
	"ds2api/internal/config"
	"ds2api/internal/deepseek"
)

type AuthResolver interface {
	Determine(req *http.Request) (*auth.RequestAuth, error)
	DetermineCaller(req *http.Request) (*auth.RequestAuth, error)
	Release(a *auth.RequestAuth)
}

type DeepSeekCaller interface {
	CreateSession(ctx context.Context, a *auth.RequestAuth, maxAttempts int) (string, error)
	GetPow(ctx context.Context, a *auth.RequestAuth, maxAttempts int) (string, error)
	UploadFile(ctx context.Context, a *auth.RequestAuth, req deepseek.UploadFileRequest, maxAttempts int) (*deepseek.UploadFileResult, error)
	CallCompletion(ctx context.Context, a *auth.RequestAuth, payload map[string]any, powResp string, maxAttempts int) (*http.Response, error)
	DeleteSessionForToken(ctx context.Context, token string, sessionID string) (*deepseek.DeleteSessionResult, error)
	DeleteAllSessionsForToken(ctx context.Context, token string) error
}

type ConfigReader interface {
	ModelAliases() map[string]string
	CompatWideInputStrictOutput() bool
	CompatStripReferenceMarkers() bool
	ToolcallMode() string
	ToolcallEarlyEmitConfidence() string
	ResponsesStoreTTLSeconds() int
	EmbeddingsProvider() string
	AutoDeleteMode() string
	AutoDeleteSessions() bool
	AcquireAutoDeleteAllLease(accountID string) bool
}

var _ AuthResolver = (*auth.Resolver)(nil)
var _ DeepSeekCaller = (*deepseek.Client)(nil)
var _ ConfigReader = (*config.Store)(nil)
