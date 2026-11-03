package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func TestEnsureRuntimeLLMBindingsSeedsWhenEmpty(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	runtimeID, runtimeOwnerID, _ := runtimeVisibilityFixture(t)
	rtUUID := util.MustParseUUID(runtimeID)

	modelA, err := testQueries.CreateLLMModel(ctx, db.CreateLLMModelParams{
		ModelID:   "rt-bind-a-" + runtimeID[:8],
		Label:     "A",
		Enabled:   true,
		SortOrder: 1,
	})
	if err != nil {
		t.Fatalf("create model a: %v", err)
	}
	t.Cleanup(func() { _ = testQueries.DeleteLLMModel(ctx, modelA.ID) })

	modelB, err := testQueries.CreateLLMModel(ctx, db.CreateLLMModelParams{
		ModelID:   "rt-bind-b-" + runtimeID[:8],
		Label:     "B",
		Enabled:   false,
		SortOrder: 2,
	})
	if err != nil {
		t.Fatalf("create model b: %v", err)
	}
	t.Cleanup(func() { _ = testQueries.DeleteLLMModel(ctx, modelB.ID) })

	_ = testQueries.DeleteRuntimeLLMModelsByRuntime(ctx, rtUUID)
	testHandler.ensureRuntimeLLMBindings(ctx, testQueries, rtUUID)

	ids, err := testQueries.ListRuntimeLLMModelIDs(ctx, rtUUID)
	if err != nil {
		t.Fatalf("list bindings: %v", err)
	}
	foundA, foundB := false, false
	for _, id := range ids {
		if id == modelA.ModelID {
			foundA = true
		}
		if id == modelB.ModelID {
			foundB = true
		}
	}
	if !foundA {
		t.Fatalf("expected enabled model %q in bindings, got %#v", modelA.ModelID, ids)
	}
	if foundB {
		t.Fatalf("disabled model %q should not be seeded, got %#v", modelB.ModelID, ids)
	}

	// Second ensure must not wipe operator removals.
	_ = testQueries.DeleteRuntimeLLMModel(ctx, db.DeleteRuntimeLLMModelParams{
		RuntimeID: rtUUID,
		ModelID:   modelA.ModelID,
	})
	testHandler.ensureRuntimeLLMBindings(ctx, testQueries, rtUUID)
	ids, err = testQueries.ListRuntimeLLMModelIDs(ctx, rtUUID)
	if err != nil {
		t.Fatalf("list after second seed: %v", err)
	}
	for _, id := range ids {
		if id == modelA.ModelID {
			t.Fatalf("second ensure re-added removed binding")
		}
	}

	_ = runtimeOwnerID // keep fixture owner used by PATCH below
}

func TestUpdateAgentRuntime_ServerModelIDs(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	runtimeID, runtimeOwnerID, _ := runtimeVisibilityFixture(t)
	rtUUID := util.MustParseUUID(runtimeID)

	model, err := testQueries.CreateLLMModel(ctx, db.CreateLLMModelParams{
		ModelID:   "rt-patch-" + runtimeID[:8],
		Label:     "Patch",
		Enabled:   true,
		SortOrder: 0,
	})
	if err != nil {
		t.Fatalf("create model: %v", err)
	}
	t.Cleanup(func() {
		_ = testQueries.DeleteRuntimeLLMModelsByRuntime(ctx, rtUUID)
		_ = testQueries.DeleteLLMModel(ctx, model.ID)
	})

	w := patchRuntimeCustomName(runtimeOwnerID, runtimeID, map[string]any{
		"server_model_ids": []string{model.ModelID},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("set bindings: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp AgentRuntimeResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.ServerModelIDs) != 1 || resp.ServerModelIDs[0] != model.ModelID {
		t.Fatalf("server_model_ids=%#v want [%q]", resp.ServerModelIDs, model.ModelID)
	}

	w = patchRuntimeCustomName(runtimeOwnerID, runtimeID, map[string]any{
		"server_model_ids": []string{"not-a-real-model"},
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("unknown model: expected 400, got %d: %s", w.Code, w.Body.String())
	}

	w = patchRuntimeCustomName(runtimeOwnerID, runtimeID, map[string]any{
		"server_model_ids": []string{},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("clear bindings: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode clear: %v", err)
	}
	if len(resp.ServerModelIDs) != 0 {
		t.Fatalf("cleared bindings want [], got %#v", resp.ServerModelIDs)
	}
}

func TestInjectServerLLMEnv_RequiresRuntimeBinding(t *testing.T) {
	if testHandler == nil {
		t.Skip("database not available")
	}
	ctx := context.Background()
	runtimeID, runtimeOwnerID, _ := runtimeVisibilityFixture(t)
	rtUUID := util.MustParseUUID(runtimeID)

	model, err := testQueries.CreateLLMModel(ctx, db.CreateLLMModelParams{
		ModelID:   "rt-inject-" + runtimeID[:8],
		Label:     "Inject",
		Enabled:   true,
		SortOrder: 0,
	})
	if err != nil {
		t.Fatalf("create model: %v", err)
	}
	t.Cleanup(func() {
		_ = testQueries.DeleteRuntimeLLMModelsByRuntime(ctx, rtUUID)
		_ = testQueries.DeleteLLMModel(ctx, model.ID)
	})

	// Upsert gateway config so inject has something to write when allowed.
	_, _ = testQueries.UpsertLLMConfig(ctx, db.UpsertLLMConfigParams{
		ID:              llmConfigID,
		BaseUrl:         "https://example.test/v1",
		ApiKeyEncrypted: nil,
		UpdatedBy:       util.MustParseUUID(runtimeOwnerID),
	})

	env := testHandler.injectServerLLMEnv(ctx, rtUUID, model.ModelID, nil)
	if _, ok := env["OPENAI_BASE_URL"]; ok {
		t.Fatalf("inject without binding should be no-op, got %#v", env)
	}

	if err := testQueries.InsertRuntimeLLMModel(ctx, db.InsertRuntimeLLMModelParams{
		RuntimeID: rtUUID,
		ModelID:   model.ModelID,
	}); err != nil {
		t.Fatalf("bind: %v", err)
	}
	env = testHandler.injectServerLLMEnv(ctx, rtUUID, model.ModelID, nil)
	if env["OPENAI_BASE_URL"] != "https://example.test/v1" {
		t.Fatalf("inject with binding: OPENAI_BASE_URL=%q", env["OPENAI_BASE_URL"])
	}
}
