package consumer

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/types/known/structpb"

	pluginv1 "github.com/ContinuumApp/continuum-plugin-sdk/pkg/pluginproto/silo/plugin/v1"

	"github.com/RXWatcher/silo-plugin-ebooks/internal/migrate"
	"github.com/RXWatcher/silo-plugin-ebooks/internal/store"
)

func newConsumerStore(t *testing.T) *store.Store {
	t.Helper()
	base := os.Getenv("TEST_DATABASE_URL")
	if base == "" {
		t.Skip("TEST_DATABASE_URL unset")
	}
	schema := fmt.Sprintf("consumer_test_%d", os.Getpid())
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, base)
	if err != nil {
		t.Skipf("postgres unreachable: %v", err)
	}
	_, _ = admin.Exec(ctx, "DROP SCHEMA IF EXISTS "+schema+" CASCADE")
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	admin.Close()
	u, _ := url.Parse(base)
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	dsn := u.String()
	if err := migrate.Run(ctx, dsn); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return store.New(pool)
}

// A foreign request_id (these backends are shared with other installs) must
// be ACKed and dropped, not nacked forever (poison message / redelivery DoS).
func TestHandleEvent_ForeignRequestIDAcked(t *testing.T) {
	st := newConsumerStore(t)
	h := New(func() *Deps { return &Deps{Store: st} })

	payload, _ := structpb.NewStruct(map[string]any{
		"request_id":        "not-a-request-in-this-install",
		"fulfilled_book_id": "bk-9",
	})
	resp, err := h.HandleEvent(context.Background(), &pluginv1.HandleEventRequest{
		EventName: "plugin.silo.bookwarehouse-ebook.request_fulfilled",
		Payload:   payload,
	})
	if err != nil {
		t.Fatalf("foreign event nacked (poison message): %v", err)
	}
	if resp == nil {
		t.Fatal("nil response")
	}
}

func TestHandleEvent_NilDepsFnNacks(t *testing.T) {
	h := New(nil)
	payload, _ := structpb.NewStruct(map[string]any{
		"request_id":  "r-1",
		"external_id": "ext-1",
	})
	resp, err := h.HandleEvent(context.Background(), &pluginv1.HandleEventRequest{
		EventName: "plugin.silo.bookwarehouse-ebook.request_acknowledged",
		Payload:   payload,
	})
	if err == nil {
		t.Fatal("nil depsFn must return an error so the host redelivers")
	}
	if resp != nil {
		t.Fatalf("response = %+v, want nil on nack", resp)
	}
}
