package server

import (
	"context"
	"testing"

	"github.com/RXWatcher/silo-plugin-ebooks/internal/store"
)

type fakeEventPublisher struct {
	target  string
	name    string
	payload map[string]any
}

func (f *fakeEventPublisher) Publish(_ context.Context, name string, payload map[string]any) {
	f.name = name
	f.payload = payload
}

type fakeTargetedEventPublisher struct {
	fakeEventPublisher
}

func (f *fakeTargetedEventPublisher) PublishTo(_ context.Context, targetPluginID, name string, payload map[string]any) {
	f.target = targetPluginID
	f.name = name
	f.payload = payload
}

func TestPublishRequestSubmittedUsesTargetedPublisher(t *testing.T) {
	pub := &fakeTargetedEventPublisher{}
	publishRequestSubmitted(context.Background(), pub, store.Request{
		ID:             "req-1",
		Title:          "T",
		Authors:        []string{"A"},
		TargetPluginID: "  silo.bookwarehouse-ebook  ",
		MediaType:      "book",
	})

	if pub.target != "silo.bookwarehouse-ebook" {
		t.Fatalf("target = %q", pub.target)
	}
	if pub.name != "request_submitted" {
		t.Fatalf("name = %q", pub.name)
	}
	if pub.payload["target_plugin_id"] != "silo.bookwarehouse-ebook" {
		t.Fatalf("payload target = %+v", pub.payload)
	}
	if pub.payload["target_provider_plugin_id"] != "silo.bookwarehouse-ebook" {
		t.Fatalf("compat target = %+v", pub.payload)
	}
}

func TestPublishRequestSubmittedFallsBackToBroadcast(t *testing.T) {
	pub := &fakeEventPublisher{}
	publishRequestSubmitted(context.Background(), pub, store.Request{
		ID:             "req-1",
		TargetPluginID: "silo.ebook-requests",
	})

	if pub.name != "request_submitted" {
		t.Fatalf("name = %q", pub.name)
	}
	if pub.payload["target_plugin_id"] != "silo.ebook-requests" {
		t.Fatalf("payload target = %+v", pub.payload)
	}
}
