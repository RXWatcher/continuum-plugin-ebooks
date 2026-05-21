package recommend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/pgvector/pgvector-go"

	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/backend"
	"github.com/ContinuumApp/continuum-plugin-ebooks/internal/store"
)

// Logger is the narrow logging surface — hclog + slog both satisfy.
type Logger interface {
	Debug(msg string, args ...any)
	Warn(msg string, args ...any)
}

type noopLogger struct{}

func (noopLogger) Debug(string, ...any) {}
func (noopLogger) Warn(string, ...any)  {}

// Engine is the long-lived recommend service for ebooks. Same
// surface as the audiobooks plugin's Engine — Configured() gates,
// EmbedAndStore lock-in optimisation, Similar with caching, and a
// BackfillLibrary walker.
type Engine struct {
	client *Client
	store  *store.Store
	logger Logger
}

func New(cfg ClientConfig, st *store.Store, logger Logger) *Engine {
	if logger == nil {
		logger = noopLogger{}
	}
	return &Engine{
		client: NewClient(cfg),
		store:  st,
		logger: logger,
	}
}

func (e *Engine) Configured() bool {
	return e != nil && e.client.Configured() && e.store != nil
}

// EmbedAndStore generates an embedding for one ebook detail and
// writes it. Skipped (no-op) when the canonical text + model match
// the stored row. Idempotent across calls.
func (e *Engine) EmbedAndStore(ctx context.Context, libraryID int64, d backend.EbookDetail) error {
	if !e.Configured() {
		return nil
	}
	canonicalText := BuildEmbeddingText(d)
	if existing, err := e.store.GetEbookEmbedding(ctx, libraryID, d.ID); err == nil {
		if existing.CanonicalText == canonicalText && existing.Model == e.client.Model() {
			e.logger.Debug("recommend: embedding unchanged",
				"library_id", libraryID, "book_id", d.ID)
			return nil
		}
	}
	vecs, err := e.client.Embed(ctx, []string{canonicalText})
	if err != nil {
		return fmt.Errorf("embed: %w", err)
	}
	if len(vecs) == 0 || len(vecs[0]) == 0 {
		return errors.New("embed returned no vector")
	}
	return e.store.UpsertEbookEmbedding(ctx, store.EbookEmbedding{
		BookID:        d.ID,
		LibraryID:     libraryID,
		Embedding:     pgvector.NewVector(vecs[0]),
		Model:         e.client.Model(),
		CanonicalText: canonicalText,
	})
}

// Similar returns up to `limit` similar ebook ids for the given
// source, using the cached entry when fresh and recomputing
// otherwise. 6h cache TTL matches the audiobooks plugin.
func (e *Engine) Similar(ctx context.Context, libraryID int64, bookID string, limit int) ([]store.SimilarEbook, error) {
	if !e.Configured() {
		return nil, nil
	}
	if cached, err := e.store.GetEbookRecommendationCache(ctx, libraryID, bookID, "similar"); err == nil {
		var items []store.SimilarEbook
		if err := json.Unmarshal(cached.Items, &items); err == nil {
			if len(items) > limit {
				items = items[:limit]
			}
			return items, nil
		}
	}
	src, err := e.store.GetEbookEmbedding(ctx, libraryID, bookID)
	if err != nil {
		e.logger.Debug("recommend: no source embedding",
			"library_id", libraryID, "book_id", bookID, "err", err.Error())
		return nil, nil
	}
	candidates, err := e.store.FindSimilarEbooks(ctx, src.Embedding, []string{bookID}, limit*3)
	if err != nil {
		return nil, fmt.Errorf("find similar: %w", err)
	}
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	if blob, err := json.Marshal(candidates); err == nil {
		_ = e.store.SetEbookRecommendationCache(ctx, libraryID, bookID, "similar", blob, 6*time.Hour)
	}
	return candidates, nil
}

// PurgeExpiredCache invokes the ebook-recommendation-cache cleanup.
// Scheduler calls this periodically.
func (e *Engine) PurgeExpiredCache(ctx context.Context) (int, error) {
	if e.store == nil {
		return 0, nil
	}
	return e.store.PurgeExpiredEbookRecommendations(ctx)
}

// LoadConfigFromEnv reads the same env-var triple the audiobooks
// plugin uses (EMBEDDING_BASE_URL / EMBEDDING_MODEL /
// EMBEDDING_API_KEY) so an operator running both plugins points them
// at the same embedding endpoint with one config block.
func LoadConfigFromEnv(get func(string) string) ClientConfig {
	return ClientConfig{
		BaseURL: strings.TrimSpace(get("EMBEDDING_BASE_URL")),
		Model:   strings.TrimSpace(get("EMBEDDING_MODEL")),
		APIKey:  strings.TrimSpace(get("EMBEDDING_API_KEY")),
	}
}
