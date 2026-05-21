// Package consumer implements the event_consumer.v1 handler that processes
// backend-emitted events (request_acknowledged/status_changed/fulfilled/failed)
// from ebook request providers.
package consumer

import (
	"context"
	"errors"
	"fmt"

	pluginv1 "github.com/ContinuumApp/continuum-plugin-sdk/pkg/pluginproto/continuum/plugin/v1"

	"github.com/RXWatcher/continuum-plugin-ebooks/internal/store"
)

type Deps struct {
	Store *store.Store
}

type Handler struct {
	pluginv1.UnimplementedEventConsumerServer
	depsFn func() *Deps
}

func New(depsFn func() *Deps) *Handler { return &Handler{depsFn: depsFn} }

func (h *Handler) HandleEvent(ctx context.Context, req *pluginv1.HandleEventRequest) (*pluginv1.HandleEventResponse, error) {
	if h.depsFn == nil {
		return nil, fmt.Errorf("plugin not configured yet")
	}
	d := h.depsFn()
	if d == nil {
		// Capability servers serve before Configure runs. Nack so the host
		// redelivers once configured, instead of acking and dropping the event.
		return nil, fmt.Errorf("plugin not configured yet")
	}
	if req.GetPayload() == nil {
		return &pluginv1.HandleEventResponse{}, nil // malformed; redelivery won't help
	}
	p := req.GetPayload().AsMap()
	requestID := requestIDFromPayload(p)
	if requestID == "" {
		return &pluginv1.HandleEventResponse{}, nil
	}
	externalID, _ := p["external_id"].(string)

	name := req.GetEventName()
	// Trim the "plugin.<source>." prefix to find the leaf event name.
	leaf := name
	for i := len(leaf) - 1; i >= 0; i-- {
		if leaf[i] == '.' {
			leaf = leaf[i+1:]
			break
		}
	}
	var err error
	switch leaf {
	case "request_acknowledged":
		err = d.Store.AdvanceRequestStatus(ctx, requestID, "acknowledged", externalID, "", "")
	case "request_status_changed":
		status, _ := p["status"].(string)
		if status == "" {
			return &pluginv1.HandleEventResponse{}, nil // nothing to apply
		}
		err = d.Store.AdvanceRequestStatus(ctx, requestID, status, externalID, "", "")
	case "request_fulfilled":
		bookID, _ := p["fulfilled_book_id"].(string)
		err = d.Store.AdvanceRequestStatus(ctx, requestID, "fulfilled", externalID, "", bookID)
	case "request_failed":
		reason, _ := p["reason"].(string)
		err = d.Store.AdvanceRequestStatus(ctx, requestID, "failed", externalID, reason, "")
	default:
		return &pluginv1.HandleEventResponse{}, nil // unknown event; ack
	}
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			// The request_id is not ours: these backends are shared and also
			// serve other portals/installs, whose request_ids never exist
			// here. Nacking would redeliver this foreign event forever (a
			// poison message that never drains). Ack-drop it. A rare genuine
			// "row not committed yet" race self-heals because the backend's
			// reconciler periodically re-emits the status.
			return &pluginv1.HandleEventResponse{}, nil
		}
		// Real DB/transient error — nack so the host redelivers rather than
		// losing a terminal status transition.
		return nil, fmt.Errorf("handle %s: %w", leaf, err)
	}
	return &pluginv1.HandleEventResponse{}, nil
}

func requestIDFromPayload(p map[string]any) string {
	if id, _ := p["request_id"].(string); id != "" {
		return id
	}
	id, _ := p["requestId"].(string)
	return id
}
