package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"time"

	"setec-manager/internal/config"
	"setec-manager/internal/db"
	"setec-manager/internal/float"
	"setec-manager/internal/hosting"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
)

type Server struct {
	Config         *config.Config
	DB             *db.DB
	Router         *chi.Mux
	http           *http.Server
	JWTKey         []byte
	FloatBridge    *float.Bridge
	HostingConfigs *hosting.ProviderConfigStore
}

func New(cfg *config.Config, database *db.DB, jwtKey []byte) *Server {
	// Initialize hosting provider config store.
	hostingDir := filepath.Join(filepath.Dir(cfg.Database.Path), "hosting")
	hostingConfigs := hosting.NewConfigStore(hostingDir)

	s := &Server{
		Config:         cfg,
		DB:             database,
		Router:         chi.NewRouter(),
		JWTKey:         jwtKey,
		FloatBridge:    float.NewBridge(database),
		HostingConfigs: hostingConfigs,
	}

	s.setupMiddleware()
	s.setupRoutes()
	return s
}

func (s *Server) setupMiddleware() {
	s.Router.Use(chiMiddleware.RequestID)
	s.Router.Use(chiMiddleware.RealIP)
	s.Router.Use(chiMiddleware.Logger)
	s.Router.Use(chiMiddleware.Recoverer)
	s.Router.Use(chiMiddleware.Timeout(60 * time.Second))
	s.Router.Use(securityHeaders)
	s.Router.Use(maxBodySize(10 << 20)) // 10MB max request body
	s.Router.Use(csrfProtection)
}

func (s *Server) Start() error {
	addr := fmt.Sprintf("%s:%d", s.Config.Server.Host, s.Config.Server.Port)

	s.http = &http.Server{
		Addr:         addr,
		Handler:      s.Router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	log.Printf("[setec] Starting on %s (TLS=%v)", addr, s.Config.Server.TLS)

	if s.Config.Server.TLS {
		return s.http.ListenAndServeTLS(s.Config.Server.Cert, s.Config.Server.Key)
	}
	return s.http.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.http.Shutdown(ctx)
}
