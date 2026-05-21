-- Embedding-based "similar books" recommendations for ebooks.
-- pgvector is required at runtime. The DO block below catches the
-- "extension not available" error so dev/test environments without
-- pgvector installed don't break the rest of the migration sequence
-- (the recommend.Engine detects the missing table at runtime and
-- silently no-ops). Production deployments MUST install pgvector
-- before this migration runs in order to enable the similar-items
-- surface.
DO $$
BEGIN
  CREATE EXTENSION IF NOT EXISTS vector;

  EXECUTE $sql$
    CREATE TABLE IF NOT EXISTS ebook_embedding (
      book_id        TEXT NOT NULL,
      library_id     BIGINT NOT NULL,
      embedding      VECTOR(1536) NOT NULL,
      model          TEXT NOT NULL,
      canonical_text TEXT NOT NULL DEFAULT '',
      created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
      updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
      PRIMARY KEY (book_id, library_id)
    )
  $sql$;

  EXECUTE $sql$
    CREATE INDEX IF NOT EXISTS ebook_embedding_hnsw_idx
      ON ebook_embedding USING hnsw (embedding vector_cosine_ops)
  $sql$;

  EXECUTE $sql$
    CREATE TABLE IF NOT EXISTS ebook_recommendation_cache (
      book_id    TEXT NOT NULL,
      library_id BIGINT NOT NULL,
      rec_type   TEXT NOT NULL,
      items      JSONB NOT NULL,
      expires_at TIMESTAMPTZ NOT NULL,
      created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
      PRIMARY KEY (book_id, library_id, rec_type)
    )
  $sql$;

  EXECUTE $sql$
    CREATE INDEX IF NOT EXISTS ebook_recommendation_cache_expires_idx
      ON ebook_recommendation_cache (expires_at)
  $sql$;
EXCEPTION WHEN OTHERS THEN
  RAISE NOTICE 'pgvector extension not available; ebook embedding tables skipped (recommend.Engine will no-op)';
END $$;
