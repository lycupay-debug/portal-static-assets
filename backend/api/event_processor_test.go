package main

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

func TestEventProcessorSupportedTypes(t *testing.T) {
	processor := EventProcessor{
		Handlers: map[string]EventHandler{
			"content.updated": func(context.Context, EventInput) (json.RawMessage, error) { return nil, nil },
			"content.created": func(context.Context, EventInput) (json.RawMessage, error) { return nil, nil },
			"ignored.nil":     nil,
			"":                func(context.Context, EventInput) (json.RawMessage, error) { return nil, nil },
		},
	}
	got := processor.supportedTypes()
	want := []string{"content.created", "content.updated"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("supportedTypes() = %#v, want %#v", got, want)
	}
}

func TestEventProcessorWithoutHandlersDoesNotClaim(t *testing.T) {
	processor := EventProcessor{}
	if got := processor.supportedTypes(); len(got) != 0 {
		t.Fatalf("supportedTypes() = %#v, want empty", got)
	}
	// An empty registry is intentionally inert: unknown event types must not be
	// marked completed or failed just because no business handler is registered.
	processor.Run(context.Background())
}

func TestEventProcessorWorkerCountBounds(t *testing.T) {
	for _, test := range []struct {
		name  string
		input int
		want  int
	}{
		{name: "default", input: 0, want: defaultProcessingWorkers},
		{name: "negative uses default", input: -1, want: defaultProcessingWorkers},
		{name: "one worker", input: 1, want: 1},
		{name: "maximum", input: maxProcessingWorkers, want: maxProcessingWorkers},
		{name: "clamped", input: maxProcessingWorkers + 1, want: maxProcessingWorkers},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := normalizeEventProcessorWorkers(test.input); got != test.want {
				t.Fatalf("normalizeEventProcessorWorkers(%d) = %d, want %d", test.input, got, test.want)
			}
		})
	}
}
