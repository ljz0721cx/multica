-- Per-runtime allowlist of instance-level server LLM models (model_id text
-- matches llm_model.model_id). No FK/cascade: app code cleans up on runtime
-- delete. Unique (runtime_id, model_id) is built CONCURRENTLY in 609.
CREATE TABLE IF NOT EXISTS runtime_llm_model (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    runtime_id UUID NOT NULL,
    model_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
