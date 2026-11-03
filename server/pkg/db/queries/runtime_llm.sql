-- name: ListEnabledLLMModelIDs :many
SELECT model_id FROM llm_model
WHERE enabled = true
ORDER BY sort_order ASC, created_at ASC;

-- name: CountRuntimeLLMModels :one
SELECT count(*)::bigint FROM runtime_llm_model
WHERE runtime_id = $1;

-- name: ListRuntimeLLMModelIDs :many
SELECT model_id FROM runtime_llm_model
WHERE runtime_id = $1
ORDER BY created_at ASC, model_id ASC;

-- name: HasRuntimeLLMModel :one
SELECT EXISTS(
    SELECT 1 FROM runtime_llm_model
    WHERE runtime_id = $1 AND model_id = $2
)::bool AS exists;

-- name: InsertRuntimeLLMModel :exec
INSERT INTO runtime_llm_model (runtime_id, model_id)
VALUES ($1, $2)
ON CONFLICT (runtime_id, model_id) DO NOTHING

-- name: DeleteRuntimeLLMModelsByRuntime :exec
DELETE FROM runtime_llm_model WHERE runtime_id = $1;

-- name: DeleteRuntimeLLMModel :exec
DELETE FROM runtime_llm_model
WHERE runtime_id = $1 AND model_id = $2;
