// Package scheduler runs cron-scheduled HTTP jobs loaded from the repository.
package scheduler

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"text/template"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/pschlump/httpcron/lib/repository"
)

// Scheduler loads events from the repository and fires HTTP requests on their cron schedules.
type Scheduler struct {
	repo   *repository.Repository
	log    *slog.Logger
	client *http.Client
}

// New creates a Scheduler.
func New(repo *repository.Repository, log *slog.Logger) *Scheduler {
	return &Scheduler{
		repo: repo,
		log:  log,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Start loads all events, registers them with robfig/cron, runs the scheduler,
// and blocks until ctx is cancelled.  When ctx is done it stops the scheduler
// and waits for any in-flight jobs to finish before returning.
func (s *Scheduler) Start(ctx context.Context) error {
	events, err := s.repo.ListAllEvents(ctx)
	if err != nil {
		return err
	}

	// WithSeconds enables the 6-field format (S M H DOM Month DOW) that
	// matches the output of englishtocron (7-field; we strip the Year below).
	c := cron.New(cron.WithSeconds())

	s.log.Info("scheduler: loading events", "count", len(events))
	for _, ev := range events {
		ev := ev // pin loop variable for the closure

		spec := normalizeCronSpec(ev.CronSpec)
		if spec == "" {
			s.log.Warn("scheduler: skipping event with no cron_spec",
				"event_id", ev.EventID, "event_name", ev.EventName)
			continue
		}
		if ev.URL == "" {
			s.log.Warn("scheduler: skipping event with no url",
				"event_id", ev.EventID, "event_name", ev.EventName)
			continue
		}

		if _, err := c.AddFunc(spec, func() { s.runJob(ev) }); err != nil {
			s.log.Error("scheduler: invalid cron_spec",
				"event_id", ev.EventID, "spec", spec, "err", err)
		} else {
			s.log.Info("scheduler: registered job",
				"event_id", ev.EventID, "event_name", ev.EventName, "spec", spec)
		}
	}

	c.Start()
	<-ctx.Done()

	// Stop accepts no new jobs and waits for running ones to complete.
	stopCtx := c.Stop()
	<-stopCtx.Done()
	return nil
}

// normalizeCronSpec converts the 7-field englishtocron output
// (S M H DOM Month DOW Year) to the 6-field format expected by
// robfig/cron/v3 with WithSeconds() (S M H DOM Month DOW).
// It also replaces '?' (Quartz-style wildcard) with '*'.
func normalizeCronSpec(spec string) string {
	fields := strings.Fields(spec)
	if len(fields) == 7 {
		fields = fields[:6]
	}
	result := strings.Join(fields, " ")
	return strings.ReplaceAll(result, "?", "*")
}

// jobData is the data available inside a body_template.
type jobData struct {
	EventID      string
	UserID       string
	EventName    string
	CronSpec     string
	HumanSpec    string
	URL          string
	HTTPMethod   string
	FiredAt      time.Time
}

func (s *Scheduler) runJob(ev repository.UserEvent) {
	fired := time.Now()

	data := jobData{
		EventID:    ev.EventID,
		UserID:     ev.UserID,
		EventName:  ev.EventName,
		CronSpec:   ev.CronSpec,
		HumanSpec:  ev.HumanSpec,
		URL:        ev.URL,
		HTTPMethod: ev.HTTPMethod,
		FiredAt:    fired,
	}

	// Render body template.
	tmpl, err := template.New("body").Parse(ev.BodyTemplate)
	if err != nil {
		s.log.Error("scheduler: parse body_template",
			"event_id", ev.EventID, "err", err)
		return
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		s.log.Error("scheduler: execute body_template",
			"event_id", ev.EventID, "err", err)
		return
	}

	method := strings.ToUpper(ev.HTTPMethod)
	if method == "" {
		method = http.MethodPost
	}

	// GET and HEAD have no body by convention.
	var body io.Reader
	if method != http.MethodGet && method != http.MethodHead {
		body = &buf
	}

	req, err := http.NewRequest(method, ev.URL, body)
	if err != nil {
		s.log.Error("scheduler: build request",
			"event_id", ev.EventID, "url", ev.URL, "err", err)
		return
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := s.client.Do(req)
	if err != nil {
		s.log.Error("scheduler: http request failed",
			"event_id", ev.EventID, "url", ev.URL, "method", method, "err", err)
		return
	}
	defer resp.Body.Close()
	// Drain the body so the connection can be reused.
	_, _ = io.Copy(io.Discard, resp.Body)

	s.log.Info("scheduler: job fired",
		"event_id", ev.EventID, "event_name", ev.EventName,
		"url", ev.URL, "method", method, "status", resp.StatusCode)
}
