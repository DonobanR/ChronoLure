package api

import (
	"net/http"

	mid "github.com/gophish/gophish/middleware"
	"github.com/gophish/gophish/middleware/ratelimit"
	"github.com/gophish/gophish/models"
	"github.com/gophish/gophish/worker"
	"github.com/gorilla/mux"
)

// ServerOption is an option to apply to the API server.
type ServerOption func(*Server)

// Server represents the routes and functionality of the Gophish API.
// It's not a server in the traditional sense, in that it isn't started and
// stopped. Rather, it's meant to be used as an http.Handler in the
// AdminServer.
type Server struct {
	handler             http.Handler
	worker              worker.Worker
	limiter             *ratelimit.PostLimiter
	reportingEnabled    bool
	blobDownloadEnabled bool
}

// NewServer returns a new instance of the API handler with the provided
// options applied.
func NewServer(options ...ServerOption) *Server {
	defaultWorker, _ := worker.New()
	defaultLimiter := ratelimit.NewPostLimiter()
	as := &Server{
		worker:  defaultWorker,
		limiter: defaultLimiter,
	}
	for _, opt := range options {
		opt(as)
	}
	as.registerRoutes()
	return as
}

// WithWorker is an option that sets the background worker.
func WithWorker(w worker.Worker) ServerOption {
	return func(as *Server) {
		as.worker = w
	}
}

func WithLimiter(limiter *ratelimit.PostLimiter) ServerOption {
	return func(as *Server) {
		as.limiter = limiter
	}
}

// WithReporting toggles the reporting module. When false, no reporting routes
// are registered at all (so requests get a natural 404, not a 403), giving the
// feature zero exposure surface.
func WithReporting(enabled bool) ServerOption {
	return func(as *Server) {
		as.reportingEnabled = enabled
	}
}

// WithBlobDownload toggles the render download endpoint (serving the stored
// DOCX). When false, that route is not registered.
func WithBlobDownload(enabled bool) ServerOption {
	return func(as *Server) {
		as.blobDownloadEnabled = enabled
	}
}

func (as *Server) registerRoutes() {
	root := mux.NewRouter()
	root = root.StrictSlash(true)
	router := root.PathPrefix("/api/").Subrouter()
	router.Use(mid.RequireAPIKey)
	router.Use(mid.EnforceViewOnly)
	router.HandleFunc("/imap/", as.IMAPServer)
	router.HandleFunc("/imap/validate", as.IMAPServerValidate)
	router.HandleFunc("/reset", as.Reset)
	router.HandleFunc("/campaigns/", as.Campaigns)
	router.HandleFunc("/campaigns/summary", as.CampaignsSummary)
	router.HandleFunc("/campaigns/trash", as.CampaignsTrash) // kept for backward compat
	router.HandleFunc("/campaigns/{id:[0-9]+}", as.Campaign)
	router.HandleFunc("/campaigns/{id:[0-9]+}/results", as.CampaignResults)
	// CL-102R recipient soft-delete lifecycle
	router.HandleFunc("/campaigns/{id:[0-9]+}/results/trashed", as.CampaignResultsTrashed)
	router.HandleFunc("/campaigns/{id:[0-9]+}/results/bulk-delete", as.CampaignResultsBulkDelete)
	router.HandleFunc("/campaigns/{id:[0-9]+}/results/delete-preview", as.CampaignResultsDeletePreview)
	router.HandleFunc("/campaigns/{id:[0-9]+}/results/{rid}", as.CampaignResultDelete)
	router.HandleFunc("/trash/recipient/restore-batch", as.RecipientRestoreBatch)
	router.HandleFunc("/trash/recipient/purge-batch", as.RecipientPurgeBatch)
	router.HandleFunc("/trash/recipient/{id:[0-9]+}/restore", as.RecipientRestore)
	router.HandleFunc("/trash/recipient/{id:[0-9]+}/purge", as.RecipientPurge)
	router.HandleFunc("/campaigns/{id:[0-9]+}/summary", as.CampaignSummary)
	router.HandleFunc("/campaigns/{id:[0-9]+}/complete", as.CampaignComplete)
	router.HandleFunc("/campaigns/{id:[0-9]+}/restore", as.CampaignRestore) // kept for backward compat
	router.HandleFunc("/campaigns/{id:[0-9]+}/purge", as.CampaignPurge)     // kept for backward compat
	// Global unified trash
	router.HandleFunc("/trash", as.GlobalTrash)
	router.HandleFunc("/trash/counts", as.TrashCounts)
	router.HandleFunc("/trash/recipient/batch/{batch_id}", as.RecipientBatchDetail)
	router.HandleFunc("/trash/{type}/{id:[0-9]+}/restore", as.GlobalTrashRestore)
	router.HandleFunc("/trash/{type}/{id:[0-9]+}/purge", as.GlobalTrashPurge)
	// Campaign Groups endpoints
	router.HandleFunc("/campaign-groups/", as.CampaignGroups)
	router.HandleFunc("/campaign-groups/summary", as.CampaignGroupsSummary)
	router.HandleFunc("/campaign-groups/{id:[0-9]+}", as.CampaignGroup)
	router.HandleFunc("/campaign-groups/{id:[0-9]+}/stats", as.CampaignGroupStats)
	router.HandleFunc("/campaign-groups/{id:[0-9]+}/results/trashed", as.CampaignGroupResultsTrashed)
	router.HandleFunc("/campaign-groups/{id:[0-9]+}/archive", as.CampaignGroupArchive)
	router.HandleFunc("/groups/", as.Groups)
	router.HandleFunc("/groups/summary", as.GroupsSummary)
	router.HandleFunc("/groups/{id:[0-9]+}", as.Group)
	router.HandleFunc("/groups/{id:[0-9]+}/summary", as.GroupSummary)
	router.HandleFunc("/templates/", as.Templates)
	router.HandleFunc("/templates/{id:[0-9]+}", as.Template)
	router.HandleFunc("/pages/", as.Pages)
	router.HandleFunc("/pages/{id:[0-9]+}", as.Page)
	router.HandleFunc("/smtp/", as.SendingProfiles)
	router.HandleFunc("/smtp/{id:[0-9]+}", as.SendingProfile)
	router.HandleFunc("/users/", mid.Use(as.Users, mid.RequirePermission(models.PermissionModifySystem)))
	router.HandleFunc("/users/{id:[0-9]+}", mid.Use(as.User))
	router.HandleFunc("/util/send_test_email", as.SendTestEmail)
	router.HandleFunc("/import/group", as.ImportGroup)
	router.HandleFunc("/import/email", as.ImportEmail)
	router.HandleFunc("/import/site", as.ImportSite)
	router.HandleFunc("/webhooks/", mid.Use(as.Webhooks, mid.RequirePermission(models.PermissionModifySystem)))
	router.HandleFunc("/webhooks/{id:[0-9]+}/validate", mid.Use(as.ValidateWebhook, mid.RequirePermission(models.PermissionModifySystem)))
	router.HandleFunc("/webhooks/{id:[0-9]+}", mid.Use(as.Webhook, mid.RequirePermission(models.PermissionModifySystem)))
	// Reporting routes are registered only when the feature is enabled, so a
	// standard install does not expose them at all.
	if as.reportingEnabled {
		as.registerReportRoutes(router)
	}
	as.handler = router
}

func (as *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	as.handler.ServeHTTP(w, r)
}
