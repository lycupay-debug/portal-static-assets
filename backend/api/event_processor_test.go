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
