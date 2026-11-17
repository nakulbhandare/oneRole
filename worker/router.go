package worker

import (
	"context"
	"fmt"

	"github.com/oneRole/queue"
)

// EventRouter dispatches a validated durable event to one workload handler.
// Unregistered event types are permanent contract errors and go to the DLQ.
type EventRouter struct {
	handlers map[string]EventHandler
}

func NewEventRouter(handlers map[string]EventHandler) (*EventRouter, error) {
	if len(handlers) == 0 {
		return nil, fmt.Errorf("%w: at least one event handler is required", ErrInvalidWorkerConfig)
	}
	copyOfHandlers := make(map[string]EventHandler, len(handlers))
	for eventType, handler := range handlers {
		if eventType == "" || handler == nil {
			return nil, fmt.Errorf("%w: event types and handlers must be non-empty", ErrInvalidWorkerConfig)
		}
		copyOfHandlers[eventType] = handler
	}
	return &EventRouter{handlers: copyOfHandlers}, nil
}

func (r *EventRouter) HandleEvent(ctx context.Context, event queue.Event) error {
	if r == nil {
		return &WorkFailure{Class: "EVENT_ROUTER_UNAVAILABLE"}
	}
	handler, ok := r.handlers[event.Type]
	if !ok {
		return &WorkFailure{Class: "UNSUPPORTED_EVENT_TYPE"}
	}
	return handler.HandleEvent(ctx, event)
}
