// Package scheduler runs cron-scheduled HTTP jobs loaded from the repository.
package scheduler

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/pschlump/httpcron/lib/repository"
)

// ScheduledEvent holds the cron entry ID and the event data.
type ScheduledEvent struct {
	EntryID cron.EntryID
	Event   repository.UserEvent
}

// SchedulerOp represents an operation on the scheduler (add, update, delete).
type SchedulerOp struct {
	Op      string // "add", "update", "delete"
	Event   repository.UserEvent
	Result  chan error
}

// Scheduler loads events from the repository and fires HTTP requests on their cron schedules.
type Scheduler struct {
	repo       *repository.Repository
	log        *slog.Logger
	client     *http.Client
	cron       *cron.Cron
	events     map[string]ScheduledEvent // event_id -> ScheduledEvent
	eventsMu   sync.RWMutex
	opChan     chan SchedulerOp
	started    bool
}

// New creates a Scheduler.
func New(repo *repository.Repository, log *slog.Logger) *Scheduler {
	return &Scheduler{
		repo: repo,
		log:  log,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		events: make(map[string]ScheduledEvent),
		opChan: make(chan SchedulerOp, 100),
	}
}

// Start loads all events, registers them with robfig/cron, runs the scheduler,
// and blocks until ctx is cancelled.  When ctx is done it stops the scheduler
// and waits for any in-flight jobs to finish before returning.
func (s *Scheduler) Start(ctx context.Context) error {
	// WithSeconds enables the 6-field format (S M H DOM Month DOW) that
	// matches the output of englishtocron (7-field; we strip the Year below).
	s.cron = cron.New(cron.WithSeconds())

	// Start the operation processor goroutine.
	go s.processOperations(ctx)

	// Load all events from the database.
	events, err := s.repo.ListAllEvents(ctx)
	if err != nil {
		return err
	}

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

		if entryID, err := s.cron.AddFunc(spec, func() { s.runJob(ev) }); err != nil {
			s.log.Error("scheduler: invalid cron_spec",
				"event_id", ev.EventID, "spec", spec, "err", err)
		} else {
			s.eventsMu.Lock()
			s.events[ev.EventID] = ScheduledEvent{
				EntryID: entryID,
				Event:   ev,
			}
			s.eventsMu.Unlock()
			s.log.Info("scheduler: registered job",
				"event_id", ev.EventID, "event_name", ev.EventName, "spec", spec, "entry_id", entryID)
		}
	}

	s.started = true
	s.cron.Start()
	<-ctx.Done()

	// Stop accepts no new jobs and waits for running ones to complete.
	stopCtx := s.cron.Stop()
	<-stopCtx.Done()
	return nil
}

// processOperations handles add, update, and delete operations from the channel.
func (s *Scheduler) processOperations(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case op := <-s.opChan:
			var err error
			switch op.Op {
			case "add":
				err = s.addEvent(op.Event)
			case "update":
				err = s.updateEvent(op.Event)
			case "delete":
				err = s.deleteEvent(op.Event.EventID)
			default:
				err = fmt.Errorf("unknown operation: %s", op.Op)
			}
			op.Result <- err
		}
	}
}

// AddEvent adds a new event to the running scheduler.
func (s *Scheduler) AddEvent(ctx context.Context, ev repository.UserEvent) error {
	if !s.started {
		return fmt.Errorf("scheduler not started")
	}
	result := make(chan error, 1)
	select {
	case s.opChan <- SchedulerOp{Op: "add", Event: ev, Result: result}:
		select {
		case err := <-result:
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	case <-ctx.Done():
		return ctx.Err()
	}
}

// UpdateEvent updates an existing event in the running scheduler.
func (s *Scheduler) UpdateEvent(ctx context.Context, ev repository.UserEvent) error {
	if !s.started {
		return fmt.Errorf("scheduler not started")
	}
	result := make(chan error, 1)
	select {
	case s.opChan <- SchedulerOp{Op: "update", Event: ev, Result: result}:
		select {
		case err := <-result:
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	case <-ctx.Done():
		return ctx.Err()
	}
}

// DeleteEvent removes an event from the running scheduler.
func (s *Scheduler) DeleteEvent(ctx context.Context, eventID string) error {
	if !s.started {
		return fmt.Errorf("scheduler not started")
	}
	result := make(chan error, 1)
	select {
	case s.opChan <- SchedulerOp{Op: "delete", Event: repository.UserEvent{EventID: eventID}, Result: result}:
		select {
		case err := <-result:
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	case <-ctx.Done():
		return ctx.Err()
	}
}

// addEvent adds a single event to the cron scheduler.
func (s *Scheduler) addEvent(ev repository.UserEvent) error {
	spec := normalizeCronSpec(ev.CronSpec)
	if spec == "" {
		return fmt.Errorf("invalid cron_spec")
	}
	if ev.URL == "" {
		return fmt.Errorf("url is required")
	}

	entryID, err := s.cron.AddFunc(spec, func() { s.runJob(ev) })
	if err != nil {
		return fmt.Errorf("invalid cron_spec: %w", err)
	}

	s.eventsMu.Lock()
	s.events[ev.EventID] = ScheduledEvent{
		EntryID: entryID,
		Event:   ev,
	}
	s.eventsMu.Unlock()

	s.log.Info("scheduler: added job",
		"event_id", ev.EventID, "event_name", ev.EventName, "spec", spec, "entry_id", entryID)
	return nil
}

// updateEvent updates an existing event in the cron scheduler.
func (s *Scheduler) updateEvent(ev repository.UserEvent) error {
	s.eventsMu.Lock()
	scheduled, exists := s.events[ev.EventID]
	if !exists {
		s.eventsMu.Unlock()
		return fmt.Errorf("event not found in scheduler")
	}
	s.eventsMu.Unlock()

	// Remove the old entry.
	s.cron.Remove(scheduled.EntryID)
	s.log.Info("scheduler: removed old job for update",
		"event_id", ev.EventID, "entry_id", scheduled.EntryID)

	// Add the updated event.
	return s.addEvent(ev)
}

// deleteEvent removes an event from the cron scheduler.
func (s *Scheduler) deleteEvent(eventID string) error {
	s.eventsMu.Lock()
	scheduled, exists := s.events[eventID]
	if !exists {
		s.eventsMu.Unlock()
		return fmt.Errorf("event not found in scheduler")
	}
	delete(s.events, eventID)
	s.eventsMu.Unlock()

	s.cron.Remove(scheduled.EntryID)
	s.log.Info("scheduler: removed job",
		"event_id", eventID, "entry_id", scheduled.EntryID)
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
