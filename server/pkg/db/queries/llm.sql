-- name: GetLLMConfig :one
SELECT * FROM llm_config
WHERE id = $1;

-- name: UpsertLLMConfig :one
INSERT INTO llm_config (id, base_url, api_key_encrypted, updated_by, updated_at, created_at)
VALUES ($1, $2, $3, $4, now(), now())
ON CONFLICT (id) DO UPDATE SET
    base_url = EXCLUDED.base_url,
    api_key_encrypted = COALESCE(EXCLUDED.api_key_encrypted, llm_config.api_key_encrypted),
    updated_by = EXCLUDED.updated_by,
    updated_at = now()
RETURNING *;

-- name: ListLLMModels :many
SELECT * FROM llm_model
ORDER BY sort_order ASC, created_at ASC;

-- name: ListEnabledLLMModels :many
SELECT * FROM llm_model
WHERE enabled = true
ORDER BY sort_order ASC, created_at ASC;

-- name: GetLLMModel :one
SELECT * FROM llm_model
WHERE id = $1;

-- name: GetLLMModelByModelID :one
SELECT * FROM llm_model
WHERE model_id = $1;

-- name: CreateLLMModel :one
INSERT INTO llm_model (model_id, label, enabled, sort_order)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: UpdateLLMModel :one
UPDATE llm_model SET
    model_id = COALESCE(sqlc.narg(model_id), model_id),
    label = COALESCE(sqlc.narg(label), label),
    enabled = COALESCE(sqlc.narg(enabled), enabled),
    sort_order = COALESCE(sqlc.narg(sort_order), sort_order),
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: DeleteLLMModel :exec
DELETE FROM llm_model WHERE id = $1;
