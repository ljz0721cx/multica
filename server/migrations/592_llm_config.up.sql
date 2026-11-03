-- Instance-wide OpenAI-compatible LLM gateway credentials for server-run models.
-- Application code owns the single-row invariant (id fixed); no FK / cascade.
CREATE TABLE IF NOT EXISTS llm_config (
    id UUID PRIMARY KEY,
    base_url TEXT NOT NULL DEFAULT '',
    api_key_encrypted BYTEA,
    updated_by UUID,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
