package admin

import (
	"github.com/go-chi/chi/v5"

	"ds2api/internal/chathistory"
	adminaccounts "ds2api/internal/httpapi/admin/accounts"
	adminauth "ds2api/internal/httpapi/admin/auth"
	adminconfig "ds2api/internal/httpapi/admin/configmgmt"
	admincontextplan "ds2api/internal/httpapi/admin/contextplan"
	admindevcapture "ds2api/internal/httpapi/admin/devcapture"
	adminhistory "ds2api/internal/httpapi/admin/history"
	adminproxies "ds2api/internal/httpapi/admin/proxies"
	adminrawsamples "ds2api/internal/httpapi/admin/rawsamples"
	adminsettings "ds2api/internal/httpapi/admin/settings"
	adminshared "ds2api/internal/httpapi/admin/shared"
	adminvercel "ds2api/internal/httpapi/admin/vercel"
	adminversion "ds2api/internal/httpapi/admin/version"
)

type Handler struct {
	Store       adminshared.ConfigStore
	Pool        adminshared.PoolController
	DS          adminshared.DeepSeekCaller
	OpenAI      adminshared.OpenAIChatCaller
	ChatHistory *chathistory.Store
}

func RegisterRoutes(r chi.Router, h *Handler) {
	deps := adminsharedDeps(h)
	authHandler := &adminauth.Handler{Store: deps.Store, Pool: deps.Pool, DS: deps.DS, OpenAI: deps.OpenAI, ChatHistory: deps.ChatHistory}
	accountsHandler := &adminaccounts.Handler{Store: deps.Store, Pool: deps.Pool, DS: deps.DS, OpenAI: deps.OpenAI, ChatHistory: deps.ChatHistory}
	configHandler := &adminconfig.Handler{Store: deps.Store, Pool: deps.Pool, DS: deps.DS, OpenAI: deps.OpenAI, ChatHistory: deps.ChatHistory}
	settingsHandler := &adminsettings.Handler{Store: deps.Store, Pool: deps.Pool, DS: deps.DS, OpenAI: deps.OpenAI, ChatHistory: deps.ChatHistory}
	proxiesHandler := &adminproxies.Handler{Store: deps.Store, Pool: deps.Pool, DS: deps.DS, OpenAI: deps.OpenAI, ChatHistory: deps.ChatHistory}
	rawSamplesHandler := &adminrawsamples.Handler{Store: deps.Store, Pool: deps.Pool, DS: deps.DS, OpenAI: deps.OpenAI, ChatHistory: deps.ChatHistory}
	vercelHandler := &adminvercel.Handler{Store: deps.Store, Pool: deps.Pool, DS: deps.DS, OpenAI: deps.OpenAI, ChatHistory: deps.ChatHistory}
	historyHandler := &adminhistory.Handler{Store: deps.Store, Pool: deps.Pool, DS: deps.DS, OpenAI: deps.OpenAI, ChatHistory: deps.ChatHistory}
	devCaptureHandler := &admindevcapture.Handler{Store: deps.Store, Pool: deps.Pool, DS: deps.DS, OpenAI: deps.OpenAI, ChatHistory: deps.ChatHistory}
	contextPlanHandler := &admincontextplan.Handler{}
	versionHandler := &adminversion.Handler{Store: deps.Store, Pool: deps.Pool, DS: deps.DS, OpenAI: deps.OpenAI, ChatHistory: deps.ChatHistory}

	adminauth.RegisterPublicRoutes(r, authHandler)
	r.Group(func(pr chi.Router) {
		pr.Use(authHandler.RequireAdmin)
		adminauth.RegisterProtectedRoutes(pr, authHandler)
		adminconfig.RegisterRoutes(pr, configHandler)
		adminsettings.RegisterRoutes(pr, settingsHandler)
		adminproxies.RegisterRoutes(pr, proxiesHandler)
		adminaccounts.RegisterRoutes(pr, accountsHandler)
		adminrawsamples.RegisterRoutes(pr, rawSamplesHandler)
		adminvercel.RegisterRoutes(pr, vercelHandler)
		admindevcapture.RegisterRoutes(pr, devCaptureHandler)
		admincontextplan.RegisterRoutes(pr, contextPlanHandler)
		adminhistory.RegisterRoutes(pr, historyHandler)
		adminversion.RegisterRoutes(pr, versionHandler)
	})
}

func adminsharedDeps(h *Handler) adminsharedDepsValue {
	if h == nil {
		return adminsharedDepsValue{}
	}
	return adminsharedDepsValue{Store: h.Store, Pool: h.Pool, DS: h.DS, OpenAI: h.OpenAI, ChatHistory: h.ChatHistory}
}

type adminsharedDepsValue struct {
	Store       adminshared.ConfigStore
	Pool        adminshared.PoolController
	DS          adminshared.DeepSeekCaller
	OpenAI      adminshared.OpenAIChatCaller
	ChatHistory *chathistory.Store
}
