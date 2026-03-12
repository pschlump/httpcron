// Command server starts the HTTPCron HTTP server and cron scheduler.
package main

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	httpapi "github.com/pschlump/httpcron/api"
	"github.com/pschlump/httpcron/lib/handler"
	"github.com/pschlump/httpcron/lib/repository"
	"github.com/pschlump/httpcron/lib/scheduler"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	dbPath := getenv("DB_PATH", "httpcron.db")
	addr := getenv("ADDR", ":8080")
	regKey := getenv("REGISTRATION_KEY", "dev-registration-key")

	repo, err := repository.NewRepository(dbPath)
	if err != nil {
		log.Error("open database", "err", err)
		os.Exit(1)
	}
	defer repo.Close()

	// ctx is cancelled on SIGINT/SIGTERM; both goroutines watch it.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	var wg sync.WaitGroup

	// Goroutine 1 — HTTP server.
	wg.Add(1)
	go func() {
		defer wg.Done()
		runHTTPServer(ctx, log, repo, addr, regKey)
	}()

	// Goroutine 2 — cron scheduler.
	wg.Add(1)
	go func() {
		defer wg.Done()
		sched := scheduler.New(repo, log)
		if err := sched.Start(ctx); err != nil {
			log.Error("scheduler error", "err", err)
		}
	}()

	wg.Wait()
	log.Info("shutdown complete")
}

func runHTTPServer(ctx context.Context, log *slog.Logger, repo *repository.Repository, addr, regKey string) {
	h := handler.NewHandler(repo, regKey, log)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	// API endpoints.
	r.Post("/api/v1/self-register", h.SelfRegister)
	r.Post("/api/v1/create-timed-event", h.CreateTimedEvent)
	r.Post("/api/v1/update-timed-event", h.UpdateTimedEvent)
	r.Post("/api/v1/delete-timed-event", h.DeleteTimedEvent)
	r.Post("/api/v1/list-timed-event", h.ListTimedEvent)
	r.Post("/api/v1/search-timed-event", h.SearchTimedEvent)

	// OpenAPI spec.
	r.Get("/api/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-yaml")
		_, _ = w.Write(httpapi.OpenAPISpec)
	})

	// Swagger UI — strip the "swagger-ui" directory prefix from the embedded FS.
	uiFS, err := fs.Sub(httpapi.SwaggerUIFS, "swagger-ui")
	if err != nil {
		log.Error("prepare swagger-ui fs", "err", err)
		os.Exit(1)
	}
	r.Handle("/swagger", http.RedirectHandler("/swagger/", http.StatusMovedPermanently))
	r.Handle("/swagger/*", http.StripPrefix("/swagger", http.FileServer(http.FS(uiFS))))

	srv := &http.Server{Addr: addr, Handler: r}

	// Shut down the HTTP server when ctx is cancelled.
	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutCtx); err != nil {
			log.Error("http server shutdown failed", "err", err)
		}
	}()

	log.Info("server starting", "addr", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Error("listen", "err", err)
		os.Exit(1)
	}
	log.Info("http server stopped")
}

func getenv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
