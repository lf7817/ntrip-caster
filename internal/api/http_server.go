package api

import (
	"context"
	"log/slog"
	"net/http"

	"ntrip-caster/internal/account"
	"ntrip-caster/internal/config"
	"ntrip-caster/internal/mountpoint"
)

// HTTPServer is the Admin API server.
type HTTPServer struct {
	srv *http.Server
}

// NewHTTPServer creates and configures the admin API HTTP server.
func NewHTTPServer(cfg *config.Config, acctSvc *account.Service, mgr *mountpoint.Manager) *HTTPServer {
	sess := NewSessionManager()
	h := NewHandlers(cfg, acctSvc, mgr, sess)

	mux := http.NewServeMux()

	// Public endpoints
	mux.HandleFunc("POST /api/login", h.Login)

	// Protected endpoints
	protected := http.NewServeMux()
	protected.HandleFunc("POST /api/logout", h.Logout)

	protected.HandleFunc("GET /api/users", h.ListUsers)
	protected.HandleFunc("POST /api/users", h.CreateUser)
	protected.HandleFunc("PUT /api/users/{id}", h.UpdateUser)
	protected.HandleFunc("DELETE /api/users/{id}", h.DeleteUser)

	protected.HandleFunc("GET /api/mountpoints", h.ListMountpoints)
	protected.HandleFunc("POST /api/mountpoints", h.CreateMountpoint)
	protected.HandleFunc("PUT /api/mountpoints/{id}", h.UpdateMountpoint)
	protected.HandleFunc("DELETE /api/mountpoints/{id}", h.DeleteMountpoint)

	protected.HandleFunc("GET /api/sources", h.ListSources)
	protected.HandleFunc("GET /api/clients", h.ListClients)
	protected.HandleFunc("DELETE /api/sources/{mount}", h.KickSource)
	protected.HandleFunc("DELETE /api/clients/{id}", h.KickClient)

	protected.HandleFunc("GET /api/stats", h.Stats)

	mux.Handle("/api/", sess.AuthMiddleware(protected))

	return &HTTPServer{
		srv: &http.Server{
			Addr:    cfg.Server.AdminListen,
			Handler: mux,
		},
	}
}

// ListenAndServe starts the admin HTTP server. It blocks until the server
// is shut down.
func (s *HTTPServer) ListenAndServe() error {
	slog.Info("Admin API listening", "addr", s.srv.Addr)
	err := s.srv.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

// Shutdown gracefully shuts down the admin HTTP server.
func (s *HTTPServer) Shutdown(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}
