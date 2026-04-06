package admin

import (
	"github.com/go-chi/chi/v5"
)

type Handler struct {
	Store  ConfigStore
	Pool   PoolController
	DS     DeepSeekCaller
	OpenAI OpenAIChatCaller
}

func RegisterRoutes(r chi.Router, h *Handler) {
	r.Post("/login", h.login)
	r.Get("/verify", h.verify)
	r.Group(func(pr chi.Router) {
		pr.Use(h.requireAdmin)
		pr.Get("/vercel/config", h.getVercelConfig)
		pr.Get("/config", h.getConfig)
		pr.Post("/config", h.updateConfig)
		pr.Get("/settings", h.getSettings)
		pr.Put("/settings", h.updateSettings)
		pr.Post("/settings/password", h.updateSettingsPassword)
		pr.Post("/config/import", h.configImport)
		pr.Get("/config/export", h.configExport)
		pr.Post("/keys", h.addKey)
		pr.Delete("/keys/{key}", h.deleteKey)
		pr.Get("/proxies", h.listProxies)
		pr.Post("/proxies", h.addProxy)
		pr.Put("/proxies/{proxyID}", h.updateProxy)
		pr.Delete("/proxies/{proxyID}", h.deleteProxy)
		pr.Post("/proxies/test", h.testProxy)
		pr.Get("/accounts", h.listAccounts)
		pr.Post("/accounts", h.addAccount)
		pr.Delete("/accounts/{identifier}", h.deleteAccount)
		pr.Put("/accounts/{identifier}/proxy", h.updateAccountProxy)
		pr.Get("/queue/status", h.queueStatus)
		pr.Post("/accounts/test", h.testSingleAccount)
		pr.Post("/accounts/test-all", h.testAllAccounts)
		pr.Post("/accounts/sessions/delete-all", h.deleteAllSessions)
		pr.Post("/import", h.batchImport)
		pr.Post("/test", h.testAPI)
		pr.Post("/dev/raw-samples/capture", h.captureRawSample)
		pr.Get("/dev/raw-samples/query", h.queryRawSampleCaptures)
		pr.Post("/dev/raw-samples/save", h.saveRawSampleFromCaptures)
		pr.Post("/vercel/sync", h.syncVercel)
		pr.Get("/vercel/status", h.vercelStatus)
		pr.Post("/vercel/status", h.vercelStatus)
		pr.Get("/export", h.exportConfig)
		pr.Get("/dev/captures", h.getDevCaptures)
		pr.Delete("/dev/captures", h.clearDevCaptures)
		pr.Get("/version", h.getVersion)
	})
}
