CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS runtime_llm_model_runtime_model_uidx
    ON runtime_llm_model (runtime_id, model_id);
