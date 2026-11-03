-- Catalog of models the instance makes available for agents / server LLM use.
CREATE TABLE IF NOT EXISTS llm_model (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    model_id TEXT NOT NULL,
    label TEXT NOT NULL DEFAULT '',
    enabled BOOLEAN NOT NULL DEFAULT true,
    sort_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
