package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

func (p EventProcessor) claim(ctx context.Context, eventTypes []string) (*pendingEvent, error) {
	if len(eventTypes) == 0 {
		return nil, nil
	}
	// A worker can crash on the final permitted attempt and never call fail().
	// Once that lease expires, move the event to a terminal state instead of
	// allowing repeated crashes to bypass maxProcessingAttempts indefinitely.
	_, err := p.DB.ExecContext(ctx, `
		UPDATE portal.event_inbox
		SET status = 'failed', processing_lease_until = NULL,
		    processing_worker_id = NULL,
		    last_error = 'processing lease expired after maximum attempts'
		WHERE event_type = ANY($1::text[])
		  AND status = 'processing'
		  AND processing_lease_until < now()
		  AND attempts >= $2
	`, eventTypes, maxProcessingAttempts)
	if err != nil {
		return nil, err
	}
	var event pendingEvent
	err = p.DB.QueryRowContext(ctx, `
		WITH candidate AS (
			SELECT event_id
			FROM portal.event_inbox
			WHERE event_type = ANY($1::text[])
			  AND attempts < $3
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
	`, eventTypes, p.WorkerID, maxProcessingAttempts).Scan(&event.EventID, &event.EventType, &event.Payload, &event.Attempts)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &event, nil
}

func (p EventProcessor) complete(ctx context.Context, event pendingEvent, processingResult json.RawMessage) error {
	result, err := p.DB.ExecContext(ctx, `
		UPDATE portal.event_inbox
		SET status = 'completed', processed_at = now(), processing_result = NULLIF($3, '')::jsonb,
		    processing_lease_until = NULL, processing_worker_id = NULL,
		    last_error = NULL
		WHERE event_id = $1::uuid AND status = 'processing' AND processing_worker_id = $2
	`, event.EventID, p.WorkerID, string(processingResult))
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
