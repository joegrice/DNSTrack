package server

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/joe/dnstrack/internal/api"
)

type Server struct {
	router chi.Router
	port   int
}

func New(handler *api.Handler, port int, frontendDir string) *Server {
	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RealIP)

	// API routes
	r.Mount("/api", handler.Routes())

	// Frontend handler
	r.Get("/*", serveFrontend(frontendDir))

	return &Server{router: r, port: port}
}

func (s *Server) Start() error {
	addr := fmt.Sprintf(":%d", s.port)
	log.Printf("[server] listening on %s", addr)
	return http.ListenAndServe(addr, s.router)
}

func serveFrontend(distDir string) http.HandlerFunc {
	var fileServer http.Handler

	if distDir != "" {
		if _, err := os.Stat(distDir); err == nil {
			fileServer = http.FileServer(http.Dir(distDir))
			log.Printf("[server] serving frontend from %s", distDir)
		}
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if fileServer == nil {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(`<!DOCTYPE html><html><head><title>DNSTrack</title><style>body{font-family:monospace;padding:2rem;background:#1a1a2e;color:#e0e0e0;}code{background:#16213e;padding:2px 6px;border-radius:4px;}</style></head><body><h1>🐇 DNSTrack</h1><p>Frontend not built. Run <code>cd web && npm install && npm run build</code> to build the frontend.</p><p>API available at <a href="/api/runs/latest">/api/runs/latest</a></p></body></html>`))
			return
		}

		// SPA fallback: if path doesn't match a static file, serve index.html
		path := r.URL.Path
		fullPath := filepath.Join(distDir, path)

		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			r.URL.Path = "/"
		}

		fileServer.ServeHTTP(w, r)
	}
}


