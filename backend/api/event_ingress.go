package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"time"
)

const maxEventBodyBytes = 1 << 20

var (
	eventIDPattern   = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-8][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)
	eventTypePattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,99}$`)
	errEventConflict = errors.New("event ID already exists with different content")
)

type EventInput struct {
	EventID   string          `json:"event_id"`
	EventType string          `json:"event_type"`
	Payload   json.RawMessage `json:"payload"`
}

type EventStore interface {
	ReceiveEvent(context.Context, EventInput) (duplicate bool, err error)
}

type PostgresEventStore struct{ DB *sql.DB }

func (s PostgresEventStore) ReceiveEvent(ctx context.Context, event EventInput) (bool, error) {
	var insertedID string
	err := s.DB.QueryRowContext(ctx, `
		INSERT INTO portal.event_inbox (event_id, event_type, payload)
		VALUES ($1::uuid, $2, $3::jsonb)
		ON CONFLICT (event_id) DO NOTHING
		RETURNING event_id::text`, event.EventID, event.EventType, string(event.Payload)).Scan(&insertedID)
	if err == nil {
		return false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}

	var existingType string
	var payloadMatches bool
	err = s.DB.QueryRowContext(ctx, `
		SELECT event_type, payload = $2::jsonb
		FROM portal.event_inbox WHERE event_id = $1::uuid`, event.EventID, string(event.Payload)).Scan(&existingType, &payloadMatches)
	if err != nil {
		return false, err
	}
	if existingType != event.EventType || !payloadMatches {
		return false, errEventConflict
	}
	return true, nil
}

func validateEvent(event EventInput) error {
	if !eventIDPattern.MatchString(event.EventID) {
		return errors.New("event_id must be a UUID")
	}
	if !eventTypePattern.MatchString(event.EventType) {
		return errors.New("event_type must start with a lowercase letter and contain only lowercase letters, digits, dot, underscore, or hyphen")
	}
	if len(event.Payload) == 0 || !json.Valid(event.Payload) {
		return errors.New("payload must be a JSON object")
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(event.Payload, &payload); err != nil || payload == nil {
		return errors.New("payload must be a JSON object")
	}
	return nil
}

func receiveEventHandler(store EventStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxEventBodyBytes)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		var event EventInput
		if err := decoder.Decode(&event); err != nil {
			writeError(w, http.StatusBadRequest, "invalid event request", nil)
			return
		}
		var extra any
		if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, "invalid event request", nil)
			return
		}
		if err := validateEvent(event); err != nil {
			writeError(w, http.StatusBadRequest, err.Error(), nil)
			return
		}
		// Privileged command-task events are internal-only. Public event intake must
		// never be able to create or wake a root command by choosing an event type.
		if event.EventType == commandTaskEventType {
			writeError(w, http.StatusForbidden, "privileged command events are internal-only", nil)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		duplicate, err := store.ReceiveEvent(ctx, event)
		if errors.Is(err, errEventConflict) {
			writeError(w, http.StatusConflict, "event_id conflicts with previously received content", nil)
			return
		}
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "event persistence unavailable", nil)
			return
		}
		status := http.StatusAccepted
		if duplicate {
			status = http.StatusOK
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"event_id": event.EventID, "status": "received", "duplicate": duplicate})
	}
}
