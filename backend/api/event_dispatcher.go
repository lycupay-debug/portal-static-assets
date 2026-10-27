package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type pendingNotification struct {
	EventID   string
	EventType string
	Attempts  int
}

type EventDispatcher struct {
	DB       *sql.DB
	IPC      IPCClient
	Batch    int
	Interval time.Duration
	Wake     chan<- struct{}
}

func (d EventDispatcher) claim(ctx context.Context) ([]pendingNotification, error) {
	batch := d.Batch
	if batch <= 0 || batch > 64 {
		batch = 16
	}
	rows, err := d.DB.QueryContext(ctx, `
		WITH candidates AS (
			SELECT event_id
			FROM portal.event_inbox
			WHERE ((dispatch_state = 'pending' AND next_dispatch_at <= now())
			       OR (dispatch_state = 'leased' AND dispatch_lease_until < now()))
			ORDER BY next_dispatch_at, received_at, event_id
			FOR UPDATE SKIP LOCKED
			LIMIT $1
		)
		UPDATE portal.event_inbox AS e
		SET dispatch_state = 'leased',
		    dispatch_attempts = e.dispatch_attempts + 1,
		    dispatch_lease_until = now() + interval '30 seconds',
		    dispatch_error = NULL
		FROM candidates AS c
		WHERE e.event_id = c.event_id
		RETURNING e.event_id::text, e.event_type, e.dispatch_attempts
	`, batch)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []pendingNotification
	for rows.Next() {
		var event pendingNotification
		if err := rows.Scan(&event.EventID, &event.EventType, &event.Attempts); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (d EventDispatcher) markNotified(ctx context.Context, event pendingNotification) error {
	result, err := d.DB.ExecContext(ctx, `
		UPDATE portal.event_inbox
		SET dispatch_state = 'notified', dispatch_lease_until = NULL,
		    dispatch_notified_at = now(), dispatch_error = NULL
		WHERE event_id = $1::uuid AND dispatch_state = 'leased' AND dispatch_attempts = $2
	`, event.EventID, event.Attempts)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return fmt.Errorf("event %s dispatch lease was lost before notification update", event.EventID)
	}
	return nil
}

func (d EventDispatcher) markRetry(ctx context.Context, event pendingNotification, cause error) error {
	exponent := event.Attempts - 1
	if exponent < 0 {
		exponent = 0
	}
	if exponent > 6 {
		exponent = 6
	}
	delay := time.Duration(1<<exponent) * time.Second
	message := cause.Error()
	if len(message) > 1000 {
		message = message[:1000]
	}
	result, err := d.DB.ExecContext(ctx, `
		UPDATE portal.event_inbox
		SET dispatch_state = 'pending', dispatch_lease_until = NULL,
		    next_dispatch_at = now() + ($2 * interval '1 second'),
		    dispatch_error = $3
		WHERE event_id = $1::uuid AND dispatch_state = 'leased' AND dispatch_attempts = $4
	`, event.EventID, delay.Seconds(), message, event.Attempts)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return fmt.Errorf("event %s dispatch lease was lost before retry update", event.EventID)
	}
	return nil
}

func (d EventDispatcher) Run(ctx context.Context) {
	interval := d.Interval
	if interval <= 0 {
		interval = 500 * time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		claimCtx, claimCancel := context.WithTimeout(ctx, 3*time.Second)
		events, err := d.claim(claimCtx)
		claimCancel()
		if err != nil {
			fmt.Printf("event_dispatcher_claim_failed error=%v\n", err)
		}
		if err == nil {
			for _, event := range events {
				notifyStarted := time.Now()
				if ctx.Err() != nil {
					return
				}
				notifyCtx, notifyCancel := context.WithTimeout(ctx, d.IPC.Timeout)
				notifyErr := d.IPC.PublishEvent(notifyCtx, event.EventID, event.EventType)
				notifyCancel()
				if notifyErr != nil {
					retryCtx, retryCancel := context.WithTimeout(context.Background(), 2*time.Second)
					retryErr := d.markRetry(retryCtx, event, notifyErr)
					retryCancel()
					fmt.Printf("event_dispatch_failed event_id=%s event_type=%s attempt=%d duration=%s error=%v retry_state_error=%v\n", event.EventID, event.EventType, event.Attempts, time.Since(notifyStarted).Round(time.Millisecond), notifyErr, retryErr)
					continue
				}
				markCtx, markCancel := context.WithTimeout(context.Background(), 2*time.Second)
				markErr := d.markNotified(markCtx, event)
				markCancel()
				if markErr != nil {
					fmt.Printf("event_dispatch_state_update_failed event_id=%s error=%v\n", event.EventID, markErr)
				} else {
					fmt.Printf("event_dispatch_notified event_id=%s event_type=%s attempt=%d duration=%s\n", event.EventID, event.EventType, event.Attempts, time.Since(notifyStarted).Round(time.Millisecond))
				}
				// A successful C-core acknowledgement is the fast-path wakeup.
				// Wakeups are hints only; PostgreSQL leases and recovery scans
				// guarantee that a dropped signal cannot lose a persisted event.
				if d.Wake != nil {
					select {
					case d.Wake <- struct{}{}:
					default:
					}
				}
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
