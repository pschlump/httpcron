// Command server starts the HTTPCron HTTP server and cron scheduler.
package main

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"log"
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
	"github.com/pschlump/httpcron/lib/config"
	"github.com/pschlump/httpcron/lib/handler"
	"github.com/pschlump/httpcron/lib/repository"
	"github.com/pschlump/httpcron/lib/scheduler"
)

var cfg *config.Config

var configPathFlag = flag.String("cfg", "config.json", "path of the cfg file")
var DbFlagParamFlag = flag.String("db_flag", "", "Additional Debug Flags")
var VersionFlag = flag.Bool("version", false, "Report version of code and exit")
var CommentFlag = flag.String("comment", "", "Unused comment for ps.")
var CdToFlag = flag.String("CdTo", ".", "Change directory to before running server.")

var DbOn map[string]bool
var curDir string

func main() {
	var err error

	flag.Parse()

	if *CdToFlag != "" {
		err := os.Chdir(*CdToFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Uable to chagne to %s directory, error:%s\n", *CdToFlag, err)
			os.Exit(1)
		}
	}

	if *VersionFlag {
		fmt.Printf("Version (Git Commit Version): %s Built on: %s\n", Version, BuildDate)
		os.Exit(0)
	}

	// Read config
	cfg, err = config.FromFile(*configPathFlag)
	if err != nil {
		log.Fatal(err)
	}

	cfg.Version = Version
	cfg.BuildDate = BuildDate

	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	addr := fmt.Sprintf("%s:%s", cfg.Server.Host, cfg.Server.Port) // addr := getenv("ADDR", ":8080")
	// regKey := getenv("REGISTRATION_KEY", "dev-registration-key")
	regKey := getenv("REGISTRATION_KEY", "")
	if regKey == "" {
		log.Error("REGISTRATION_KEY not set in enviroment, couerdly refusing to run.")
		os.Exit(1)
	}

	repo, err := repository.NewRepository(cfg)
	if err != nil {
		log.Error("open database", "err", err)
		os.Exit(1)
	}
	defer repo.Close()

	// ctx is cancelled on SIGINT/SIGTERM; both goroutines watch it.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	var wg sync.WaitGroup

	// Create the scheduler (but don't start it yet).
	sched := scheduler.New(repo, log)

	// Goroutine 1 — HTTP server.
	wg.Go(func() {
		runHTTPServer(ctx, log, repo, addr, regKey, sched, cfg)
	})

	// Goroutine 2 — cron scheduler.
	wg.Go(func() {
		if err := sched.Start(ctx); err != nil {
			log.Error("scheduler error", "err", err)
		}
	})

	wg.Wait()
	log.Info("shutdown complete")
}

func runHTTPServer(ctx context.Context, log *slog.Logger, repo repository.Repository, addr, regKey string, sched *scheduler.Scheduler, cfg *config.Config) {
	h := handler.NewHandler(repo, regKey, log, sched, cfg)

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
