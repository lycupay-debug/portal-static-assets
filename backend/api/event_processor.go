package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"sort"
	"time"
)

const (
	defaultProcessingInterval = 500 * time.Millisecond
	processingTimeout         = 20 * time.Second
	maxProcessingAttempts     = 10
)

type EventHandler func(context.Context, EventInput) error

// registeredEventHandlers is the single registry for production business handlers.
// Keep it empty until an event type has an explicitly defined, tested business contract.
func registeredEventHandlers() map[string]EventHandler {
	return map[string]EventHandler{}
}

type EventProcessor struct {
	DB       *sql.DB
	Handlers map[string]EventHandler
	Interval time.Duration
	WorkerID string
}

type pendingEvent struct {
	EventID   string
	EventType string
	Payload   []byte
	Attempts  int
}

func (p EventProcessor) supportedTypes() []string {
	types := make([]string, 0, len(p.Handlers))
	for eventType, handler := range p.Handlers {
		if eventType != "" && handler != nil {
			types = append(types, eventType)
		}
	}
	sort.Strings(types)
	return types
}

func (p EventProcessor) claim(ctx context.Context, eventTypes []string) (*pendingEvent, error) {
	if len(eventTypes) == 0 {
		return nil, nil
	}
	var event pendingEvent
	err := p.DB.QueryRowContext(ctx, `
		WITH candidate AS (
			SELECT event_id
			FROM portal.event_inbox
			WHERE event_type = ANY($1::text[])
			  AND (
			    (status = 'received' AND next_processing_at <= now())
			    OR (status = 'processing' AND processing_lease_until < now())
			  )
			ORDER BY next_processing_at, received_at, event_id
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		UPDATE portal.event_inbox AS e
		SET status = 'processing',
		    attempts = e.attempts + 1,
		    processing_lease_until = now() + interval '30 seconds',
		    processing_worker_id = $2,
		    last_error = NULL
		FROM candidate AS c
		WHERE e.event_id = c.event_id
		RETURNING e.event_id::text, e.event_type, e.payload::text, e.attempts
	`, eventTypes, p.WorkerID).Scan(&event.EventID, &event.EventType, &event.Payload, &event.Attempts)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &event, nil
}

func (p EventProcessor) complete(ctx context.Context, event pendingEvent) error {
	result, err := p.DB.ExecContext(ctx, `
		UPDATE portal.event_inbox
		SET status = 'completed', processed_at = now(),
		    processing_lease_until = NULL, processing_worker_id = NULL,
		    last_error = NULL
		WHERE event_id = $1::uuid AND status = 'processing' AND processing_worker_id = $2
	`, event.EventID, p.WorkerID)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return fmt.Errorf("event %s processing lease was lost before completion", event.EventID)
	}
	return nil
}

func (p EventProcessor) fail(ctx context.Context, event pendingEvent, cause error) error {
	message := cause.Error()
	if len(message) > 1000 {
		message = message[:1000]
	}
	if event.Attempts >= maxProcessingAttempts {
		result, err := p.DB.ExecContext(ctx, `
			UPDATE portal.event_inbox
			SET status = 'failed', processing_lease_until = NULL,
			    processing_worker_id = NULL, last_error = $3
			WHERE event_id = $1::uuid AND status = 'processing' AND processing_worker_id = $2
		`, event.EventID, p.WorkerID, message)
		if err != nil {
			return err
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if changed != 1 {
			return fmt.Errorf("event %s processing lease was lost before failure update", event.EventID)
		}
		return nil
	}

	exponent := event.Attempts - 1
	if exponent < 0 {
		exponent = 0
	}
	if exponent > 6 {
		exponent = 6
	}
	delay := time.Duration(1<<exponent) * time.Second
	result, err := p.DB.ExecContext(ctx, `
		UPDATE portal.event_inbox
		SET status = 'received', processing_lease_until = NULL,
		    processing_worker_id = NULL,
		    next_processing_at = now() + ($3 * interval '1 second'),
		    last_error = $4
		WHERE event_id = $1::uuid AND status = 'processing' AND processing_worker_id = $2
	`, event.EventID, p.WorkerID, delay.Seconds(), message)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return fmt.Errorf("event %s processing lease was lost before retry update", event.EventID)
	}
	return nil
}

func (p EventProcessor) processOne(ctx context.Context, eventTypes []string) bool {
	claimCtx, cancelClaim := context.WithTimeout(ctx, 3*time.Second)
	event, err := p.claim(claimCtx, eventTypes)
	cancelClaim()
	if err != nil {
		log.Printf("event processor claim failed: %v", err)
		return false
	}
	if event == nil {
		return false
	}

	handler := p.Handlers[event.EventType]
	handlerCtx, cancelHandler := context.WithTimeout(ctx, processingTimeout)
	handlerErr := handler(handlerCtx, EventInput{
		EventID: event.EventID, EventType: event.EventType, Payload: event.Payload,
	})
	cancelHandler()

	updateCtx, cancelUpdate := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancelUpdate()
	if handlerErr != nil {
		if err := p.fail(updateCtx, *event, handlerErr); err != nil {
			log.Printf("event processor failure update failed for %s: %v", event.EventID, err)
		}
		return true
	}
	if err := p.complete(updateCtx, *event); err != nil {
		log.Printf("event processor completion update failed for %s: %v", event.EventID, err)
	}
	return true
}

func (p EventProcessor) Run(ctx context.Context) {
	types := p.supportedTypes()
	if len(types) == 0 {
		return
	}
	if p.WorkerID == "" {
		p.WorkerID = fmt.Sprintf("api-%d-%d", os.Getpid(), time.Now().UnixNano())
	}
	interval := p.Interval
	if interval <= 0 {
		interval = defaultProcessingInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		if p.processOne(ctx, types) {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
