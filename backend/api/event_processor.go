package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sort"
	"sync"
	"time"
)

const (
	defaultProcessingInterval = 500 * time.Millisecond
	defaultProcessingWorkers  = 4
	maxProcessingWorkers      = 8
	processingTimeout         = 20 * time.Second
	maxProcessingAttempts     = 10
)

type EventHandler func(context.Context, EventInput) (json.RawMessage, error)

// registeredEventHandlers is the single registry for production business handlers.
// Only register handlers whose input and result contracts are explicit and tested.
func registeredEventHandlers() map[string]EventHandler {
	return map[string]EventHandler{
		"portal.content.validate": validateContentEvent,
		fileCleanupEventType:      fileCleanupEventHandler,
		commandTaskEventType:      commandTaskWakeHandler,
	}
}

func normalizeEventProcessorWorkers(workers int) int {
	if workers <= 0 {
		return defaultProcessingWorkers
	}
	if workers > maxProcessingWorkers {
		return maxProcessingWorkers
	}
	return workers
}

type EventProcessor struct {
	DB       *sql.DB
	Handlers map[string]EventHandler
	Interval time.Duration
	Workers  int
	WorkerID string
	Wake     <-chan struct{}
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

func (p EventProcessor) processOne(ctx context.Context, eventTypes []string) bool {
	claimCtx, cancelClaim := context.WithTimeout(ctx, 3*time.Second)
	event, err := p.claim(claimCtx, eventTypes)
	cancelClaim()
	if err != nil {
		if ctx.Err() == nil {
			log.Printf("event processor claim failed: %v", err)
		}
		return false
	}
	if event == nil {
		return false
	}

	handler := p.Handlers[event.EventType]
	processingStarted := time.Now()
	handlerCtx, cancelHandler := context.WithTimeout(ctx, processingTimeout)
	processingResult, handlerErr := handler(handlerCtx, EventInput{
		EventID: event.EventID, EventType: event.EventType, Payload: event.Payload,
	})
	cancelHandler()

	updateCtx, cancelUpdate := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancelUpdate()
	if handlerErr != nil {
		if err := p.fail(updateCtx, *event, handlerErr); err != nil {
			log.Printf("event_processor_failure_update_failed event_id=%s error=%v", event.EventID, err)
		} else if event.Attempts >= maxProcessingAttempts {
			log.Printf("event_dead_lettered event_id=%s event_type=%s attempts=%d error=%v", event.EventID, event.EventType, event.Attempts, handlerErr)
		}
		log.Printf("event_processed event_id=%s event_type=%s outcome=failed duration=%s error=%v", event.EventID, event.EventType, time.Since(processingStarted).Round(time.Millisecond), handlerErr)
		return true
	}
	if err := p.complete(updateCtx, *event, processingResult); err != nil {
		log.Printf("event_processor_completion_update_failed event_id=%s error=%v", event.EventID, err)
	} else {
		log.Printf("event_processed event_id=%s event_type=%s outcome=completed duration=%s", event.EventID, event.EventType, time.Since(processingStarted).Round(time.Millisecond))
	}
	return true
}

func (p EventProcessor) Run(ctx context.Context) {
	types := p.supportedTypes()
	if len(types) == 0 || ctx.Err() != nil {
		return
	}
	workers := normalizeEventProcessorWorkers(p.Workers)
	baseWorkerID := p.WorkerID
	if baseWorkerID == "" {
		baseWorkerID = fmt.Sprintf("api-%d-%d", os.Getpid(), time.Now().UnixNano())
	}
	interval := p.Interval
	if interval <= 0 {
		interval = defaultProcessingInterval
	}
	var workerGroup sync.WaitGroup
	workerGroup.Add(workers)
	for index := 0; index < workers; index++ {
		worker := p
		worker.WorkerID = fmt.Sprintf("%s-%02d", baseWorkerID, index+1)
		go func(worker EventProcessor) {
			defer workerGroup.Done()
			worker.runWorker(ctx, types, interval)
		}(worker)
	}
	workerGroup.Wait()
}

func (p EventProcessor) runWorker(ctx context.Context, eventTypes []string, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		if p.processOne(ctx, eventTypes) {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-p.Wake:
			// The C event core accepted a durable event notification. Re-scan
			// PostgreSQL immediately; the database remains the source of truth.
		case <-ticker.C:
			// Recovery scan: covers missed wakeups and process restarts.
		}
	}
}
