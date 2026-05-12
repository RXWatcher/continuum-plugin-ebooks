// Package consumer implements the event_consumer.v1 handler that processes
// backend-emitted events (request_acknowledged/status_changed/fulfilled/failed)
// for both bookwarehouse-ebook and ebookdb backends.
package consumer

import (
	"context"
	"time"

	pluginv1 "github.com/ContinuumApp/continuum-plugin-sdk/pkg/pluginproto/continuum/plugin/v1"

	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/store"
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
	d := h.depsFn()
	if d == nil || req.GetPayload() == nil {
		return &pluginv1.HandleEventResponse{}, nil
	}
	p := req.GetPayload().AsMap()
	requestID, _ := p["request_id"].(string)
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
	_ = time.Now()
	switch leaf {
	case "request_acknowledged":
		_ = d.Store.UpdateRequestStatus(ctx, requestID, "acknowledged", externalID, "", "", "")
	case "request_status_changed":
		status, _ := p["status"].(string)
		if status == "" {
			status = "acknowledged"
		}
		_ = d.Store.UpdateRequestStatus(ctx, requestID, status, externalID, "", "", "")
	case "request_fulfilled":
		bookID, _ := p["fulfilled_book_id"].(string)
		_ = d.Store.UpdateRequestStatus(ctx, requestID, "fulfilled", externalID, "", "", bookID)
	case "request_failed":
		reason, _ := p["reason"].(string)
		_ = d.Store.UpdateRequestStatus(ctx, requestID, "failed", externalID, "", reason, "")
	}
	return &pluginv1.HandleEventResponse{}, nil
}
