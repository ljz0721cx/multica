package handler

import (
	"context"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// ensureRuntimeLLMBindings seeds the runtime's server-model allowlist with
// every currently enabled llm_model when the runtime has no rows yet.
// Subsequent reconnects leave existing bindings alone so operator edits stick.
func (h *Handler) ensureRuntimeLLMBindings(ctx context.Context, q *db.Queries, runtimeID pgtype.UUID) {
	if !runtimeID.Valid {
		return
	}
	n, err := q.CountRuntimeLLMModels(ctx, runtimeID)
	if err != nil {
		slog.Warn("runtime llm: count bindings failed", "runtime_id", uuidToString(runtimeID), "error", err)
		return
	}
	if n > 0 {
		return
	}
	ids, err := q.ListEnabledLLMModelIDs(ctx)
	if err != nil {
		slog.Warn("runtime llm: list enabled models failed", "error", err)
		return
	}
	for _, modelID := range ids {
		modelID = strings.TrimSpace(modelID)
		if modelID == "" {
			continue
		}
		if err := q.InsertRuntimeLLMModel(ctx, db.InsertRuntimeLLMModelParams{
			RuntimeID: runtimeID,
			ModelID:   modelID,
		}); err != nil {
			slog.Warn("runtime llm: seed binding failed",
				"runtime_id", uuidToString(runtimeID), "model_id", modelID, "error", err)
		}
	}
}

func listRuntimeServerModelIDs(ctx context.Context, q *db.Queries, runtimeID pgtype.UUID) []string {
	if q == nil || !runtimeID.Valid {
		return []string{}
	}
	ids, err := q.ListRuntimeLLMModelIDs(ctx, runtimeID)
	if err != nil {
		slog.Warn("runtime llm: list bindings failed", "runtime_id", uuidToString(runtimeID), "error", err)
		return []string{}
	}
	if ids == nil {
		return []string{}
	}
	return ids
}

func (h *Handler) runtimeResponse(ctx context.Context, rt db.AgentRuntime) AgentRuntimeResponse {
	// Self-heal runtimes that were online before the allowlist feature shipped
	// (or never re-registered): empty allowlist gets the current enabled catalog.
	h.ensureRuntimeLLMBindings(ctx, h.Queries, rt.ID)
	resp := runtimeToResponse(rt)
	resp.ServerModelIDs = listRuntimeServerModelIDs(ctx, h.Queries, rt.ID)
	return resp
}

func (h *Handler) replaceRuntimeLLMBindings(ctx context.Context, runtimeID pgtype.UUID, modelIDs []string) error {
	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	qtx := h.Queries.WithTx(tx)
	if err := qtx.DeleteRuntimeLLMModelsByRuntime(ctx, runtimeID); err != nil {
		return err
	}
	seen := map[string]struct{}{}
	for _, raw := range modelIDs {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		if err := qtx.InsertRuntimeLLMModel(ctx, db.InsertRuntimeLLMModelParams{
			RuntimeID: runtimeID,
			ModelID:   id,
		}); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
