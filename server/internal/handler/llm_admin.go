package handler

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/multica-ai/multica/server/internal/logger"
	"github.com/multica-ai/multica/server/internal/util/secretbox"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// Fixed singleton row id for instance-wide LLM gateway config.
var llmConfigID = parseUUID("00000000-0000-4000-8000-0000000000a1")

type LLMConfigResponse struct {
	BaseURL       string `json:"base_url"`
	HasAPIKey     bool   `json:"has_api_key"`
	UpdatedAt     string `json:"updated_at,omitempty"`
	CanManage     bool   `json:"can_manage"`
}

type UpsertLLMConfigRequest struct {
	BaseURL string  `json:"base_url"`
	APIKey  *string `json:"api_key"`
}

type LLMModelResponse struct {
	ID        string `json:"id"`
	ModelID   string `json:"model_id"`
	Label     string `json:"label"`
	Enabled   bool   `json:"enabled"`
	SortOrder int32  `json:"sort_order"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type CreateLLMModelRequest struct {
	ModelID   string `json:"model_id"`
	Label     string `json:"label"`
	Enabled   *bool  `json:"enabled"`
	SortOrder *int32 `json:"sort_order"`
}

type UpdateLLMModelRequest struct {
	ModelID   *string `json:"model_id"`
	Label     *string `json:"label"`
	Enabled   *bool   `json:"enabled"`
	SortOrder *int32  `json:"sort_order"`
}

type DiscoverLLMModelItem struct {
	ModelID   string `json:"model_id"`
	Label     string `json:"label"`
	InCatalog bool   `json:"in_catalog"`
	Enabled   bool   `json:"enabled"`
}

type SyncLLMModelsRequest struct {
	ModelIDs []string `json:"model_ids"`
}

// isModelAdmin gates the model-management UI/API to operator "janle"
// (name or email local-part), optionally extended via MULTICA_MODEL_ADMIN_EMAILS.
func isModelAdmin(user db.User) bool {
	name := strings.ToLower(strings.TrimSpace(user.Name))
	email := strings.ToLower(strings.TrimSpace(user.Email))
	local, _, _ := strings.Cut(email, "@")
	if name == "janle" || local == "janle" || local == "j_janle" {
		return true
	}
	for _, allowed := range strings.Split(os.Getenv("MULTICA_MODEL_ADMIN_EMAILS"), ",") {
		allowed = strings.ToLower(strings.TrimSpace(allowed))
		if allowed != "" && allowed == email {
			return true
		}
	}
	return false
}

func (h *Handler) requireModelAdmin(w http.ResponseWriter, r *http.Request) (db.User, bool) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return db.User{}, false
	}
	user, err := h.Queries.GetUser(r.Context(), parseUUID(userID))
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return db.User{}, false
	}
	if !isModelAdmin(user) {
		writeError(w, http.StatusForbidden, "model management is restricted to admin janle")
		return db.User{}, false
	}
	return user, true
}

func llmSecretBox() (*secretbox.Box, error) {
	if key, err := secretbox.LoadKey("MULTICA_LLM_SECRET_KEY"); err == nil {
		return secretbox.New(key)
	}
	jwt := strings.TrimSpace(os.Getenv("JWT_SECRET"))
	if jwt == "" {
		return nil, errors.New("MULTICA_LLM_SECRET_KEY or JWT_SECRET required to store LLM API keys")
	}
	sum := sha256.Sum256([]byte("multica-llm-config:" + jwt))
	return secretbox.New(sum[:])
}

func llmModelToResponse(m db.LlmModel) LLMModelResponse {
	return LLMModelResponse{
		ID:        uuidToString(m.ID),
		ModelID:   m.ModelID,
		Label:     m.Label,
		Enabled:   m.Enabled,
		SortOrder: m.SortOrder,
		CreatedAt: timestampToString(m.CreatedAt),
		UpdatedAt: timestampToString(m.UpdatedAt),
	}
}

// GetLLMConfigAdmin returns gateway settings for the model admin.
func (h *Handler) GetLLMConfigAdmin(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireModelAdmin(w, r); !ok {
		return
	}
	cfg, err := h.Queries.GetLLMConfig(r.Context(), llmConfigID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusInternalServerError, "failed to load llm config")
		return
	}
	resp := LLMConfigResponse{CanManage: true}
	if err == nil {
		resp.BaseURL = cfg.BaseUrl
		resp.HasAPIKey = len(cfg.ApiKeyEncrypted) > 0
		resp.UpdatedAt = timestampToString(cfg.UpdatedAt)
	}
	writeJSON(w, http.StatusOK, resp)
}

// UpsertLLMConfigAdmin saves base URL / API key for the server LLM gateway.
func (h *Handler) UpsertLLMConfigAdmin(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireModelAdmin(w, r)
	if !ok {
		return
	}
	var req UpsertLLMConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	baseURL := strings.TrimSpace(req.BaseURL)

	var sealed []byte
	if req.APIKey != nil {
		key := strings.TrimSpace(*req.APIKey)
		if key != "" {
			box, err := llmSecretBox()
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			sealed, err = box.Seal([]byte(key))
			if err != nil {
				writeError(w, http.StatusInternalServerError, "failed to encrypt api key")
				return
			}
		} else {
			sealed = []byte{}
		}
	}

	cfg, err := h.Queries.UpsertLLMConfig(r.Context(), db.UpsertLLMConfigParams{
		ID:              llmConfigID,
		BaseUrl:         baseURL,
		ApiKeyEncrypted: sealed,
		UpdatedBy:       user.ID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save llm config")
		return
	}
	writeJSON(w, http.StatusOK, LLMConfigResponse{
		BaseURL:   cfg.BaseUrl,
		HasAPIKey: len(cfg.ApiKeyEncrypted) > 0,
		UpdatedAt: timestampToString(cfg.UpdatedAt),
		CanManage: true,
	})
}

// ListLLMModelsAdmin returns the full model catalog for the admin UI.
func (h *Handler) ListLLMModelsAdmin(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireModelAdmin(w, r); !ok {
		return
	}
	models, err := h.Queries.ListLLMModels(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list models")
		return
	}
	resp := make([]LLMModelResponse, len(models))
	for i, m := range models {
		resp[i] = llmModelToResponse(m)
	}
	writeJSON(w, http.StatusOK, resp)
}

// ListEnabledLLMModels returns enabled server models for agent pickers.
func (h *Handler) ListEnabledLLMModels(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUserID(w, r); !ok {
		return
	}
	models, err := h.Queries.ListEnabledLLMModels(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list models")
		return
	}
	resp := make([]LLMModelResponse, len(models))
	for i, m := range models {
		resp[i] = llmModelToResponse(m)
	}
	writeJSON(w, http.StatusOK, resp)
}

// CreateLLMModelAdmin adds a model to the catalog.
func (h *Handler) CreateLLMModelAdmin(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireModelAdmin(w, r); !ok {
		return
	}
	var req CreateLLMModelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	modelID := strings.TrimSpace(req.ModelID)
	if modelID == "" {
		writeError(w, http.StatusBadRequest, "model_id is required")
		return
	}
	label := strings.TrimSpace(req.Label)
	if label == "" {
		label = modelID
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	var sortOrder int32
	if req.SortOrder != nil {
		sortOrder = *req.SortOrder
	}
	m, err := h.Queries.CreateLLMModel(r.Context(), db.CreateLLMModelParams{
		ModelID:   modelID,
		Label:     label,
		Enabled:   enabled,
		SortOrder: sortOrder,
	})
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "model_id already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create model")
		return
	}
	writeJSON(w, http.StatusCreated, llmModelToResponse(m))
}

// UpdateLLMModelAdmin patches a catalog model.
func (h *Handler) UpdateLLMModelAdmin(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireModelAdmin(w, r); !ok {
		return
	}
	id, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "modelId"), "model id")
	if !ok {
		return
	}
	var req UpdateLLMModelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	m, err := h.Queries.UpdateLLMModel(r.Context(), db.UpdateLLMModelParams{
		ID:        id,
		ModelID:   req.ModelID,
		Label:     req.Label,
		Enabled:   req.Enabled,
		SortOrder: req.SortOrder,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "model not found")
			return
		}
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "model_id already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to update model")
		return
	}
	writeJSON(w, http.StatusOK, llmModelToResponse(m))
}

// DeleteLLMModelAdmin removes a catalog model.
func (h *Handler) DeleteLLMModelAdmin(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireModelAdmin(w, r); !ok {
		return
	}
	id, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "modelId"), "model id")
	if !ok {
		return
	}
	if err := h.Queries.DeleteLLMModel(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete model")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DiscoverLLMModelsAdmin lists models advertised by the configured OpenAI-
// compatible gateway so the admin can pick which ones enter the catalog.
func (h *Handler) DiscoverLLMModelsAdmin(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireModelAdmin(w, r); !ok {
		return
	}
	cfg, err := h.Queries.GetLLMConfig(r.Context(), llmConfigID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusBadRequest, "configure the LLM gateway first")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load llm config")
		return
	}
	baseURL := strings.TrimSpace(cfg.BaseUrl)
	if baseURL == "" {
		writeError(w, http.StatusBadRequest, "configure the LLM gateway base URL first")
		return
	}
	apiKey, err := h.decryptLLMAPIKey(cfg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	ids, err := fetchGatewayModelIDs(r.Context(), baseURL, apiKey)
	if err != nil {
		slog.Warn("llm discover failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	catalog, err := h.Queries.ListLLMModels(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list catalog models")
		return
	}
	byID := make(map[string]db.LlmModel, len(catalog))
	for _, m := range catalog {
		byID[m.ModelID] = m
	}
	resp := make([]DiscoverLLMModelItem, 0, len(ids))
	for _, id := range ids {
		item := DiscoverLLMModelItem{ModelID: id, Label: id}
		if m, ok := byID[id]; ok {
			item.InCatalog = true
			item.Enabled = m.Enabled
			if strings.TrimSpace(m.Label) != "" {
				item.Label = m.Label
			}
		}
		resp = append(resp, item)
	}
	writeJSON(w, http.StatusOK, resp)
}

// SyncLLMModelsAdmin enables the selected gateway model IDs in the catalog
// (creating missing rows). Unselected catalog entries are left untouched.
func (h *Handler) SyncLLMModelsAdmin(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireModelAdmin(w, r); !ok {
		return
	}
	var req SyncLLMModelsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	seen := map[string]struct{}{}
	ids := make([]string, 0, len(req.ModelIDs))
	for _, raw := range req.ModelIDs {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		writeError(w, http.StatusBadRequest, "model_ids is required")
		return
	}

	out := make([]LLMModelResponse, 0, len(ids))
	for _, id := range ids {
		existing, err := h.Queries.GetLLMModelByModelID(r.Context(), id)
		if err == nil {
			enabled := true
			m, updErr := h.Queries.UpdateLLMModel(r.Context(), db.UpdateLLMModelParams{
				ID:      existing.ID,
				Enabled: &enabled,
			})
			if updErr != nil {
				writeError(w, http.StatusInternalServerError, "failed to enable model")
				return
			}
			out = append(out, llmModelToResponse(m))
			continue
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusInternalServerError, "failed to look up model")
			return
		}
		m, createErr := h.Queries.CreateLLMModel(r.Context(), db.CreateLLMModelParams{
			ModelID:   id,
			Label:     id,
			Enabled:   true,
			SortOrder: 0,
		})
		if createErr != nil {
			if isUniqueViolation(createErr) {
				// Race with another create — re-read and enable.
				existing, getErr := h.Queries.GetLLMModelByModelID(r.Context(), id)
				if getErr != nil {
					writeError(w, http.StatusInternalServerError, "failed to create model")
					return
				}
				enabled := true
				m, updErr := h.Queries.UpdateLLMModel(r.Context(), db.UpdateLLMModelParams{
					ID:      existing.ID,
					Enabled: &enabled,
				})
				if updErr != nil {
					writeError(w, http.StatusInternalServerError, "failed to enable model")
					return
				}
				out = append(out, llmModelToResponse(m))
				continue
			}
			writeError(w, http.StatusInternalServerError, "failed to create model")
			return
		}
		out = append(out, llmModelToResponse(m))
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) decryptLLMAPIKey(cfg db.LlmConfig) (string, error) {
	if len(cfg.ApiKeyEncrypted) == 0 {
		return "", nil
	}
	box, err := llmSecretBox()
	if err != nil {
		return "", err
	}
	plain, err := box.Open(cfg.ApiKeyEncrypted)
	if err != nil {
		return "", errors.New("failed to decrypt api key")
	}
	return strings.TrimSpace(string(plain)), nil
}

func llmModelsEndpoint(baseURL string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	switch {
	case base == "":
		return ""
	case strings.HasSuffix(base, "/models"):
		return base
	case strings.HasSuffix(base, "/v1"):
		return base + "/models"
	default:
		return base + "/v1/models"
	}
}

func fetchGatewayModelIDs(ctx context.Context, baseURL, apiKey string) ([]string, error) {
	endpoint := llmModelsEndpoint(baseURL)
	if endpoint == "" {
		return nil, errors.New("base URL is empty")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gateway request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("read gateway response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(body))
		if len(msg) > 240 {
			msg = msg[:240] + "…"
		}
		if msg == "" {
			msg = resp.Status
		}
		return nil, fmt.Errorf("gateway returned %d: %s", resp.StatusCode, msg)
	}
	ids, err := parseGatewayModelIDs(body)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, errors.New("gateway returned no models")
	}
	return ids, nil
}

func parseGatewayModelIDs(raw []byte) ([]string, error) {
	var envelope struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
		Models []struct {
			ID string `json:"id"`
		} `json:"models"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		// Some gateways return a bare array of model objects.
		var arr []struct {
			ID string `json:"id"`
		}
		if err2 := json.Unmarshal(raw, &arr); err2 != nil {
			return nil, errors.New("gateway response is not a model list")
		}
		ids := make([]string, 0, len(arr))
		seen := map[string]struct{}{}
		for _, m := range arr {
			id := strings.TrimSpace(m.ID)
			if id == "" {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
		sort.Strings(ids)
		return ids, nil
	}
	src := envelope.Data
	if len(src) == 0 {
		src = envelope.Models
	}
	ids := make([]string, 0, len(src))
	seen := map[string]struct{}{}
	for _, m := range src {
		id := strings.TrimSpace(m.ID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids, nil
}


// GetModelAdminAccess tells the UI whether the current user may open model management.
func (h *Handler) GetModelAdminAccess(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	user, err := h.Queries.GetUser(r.Context(), parseUUID(userID))
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	can := isModelAdmin(user)
	writeJSON(w, http.StatusOK, map[string]any{
		"can_manage":            can,
		"can_create_workspace":  can,
		"can_create_project":    can,
	})
}

// injectServerLLMEnv overlays the instance LLM gateway credentials onto the
// agent's claim-time custom_env when the agent's bound model is an enabled
// server catalog entry. Agent-owned custom_env keys win so operators can still
// override per agent.
func (h *Handler) injectServerLLMEnv(ctx context.Context, modelID string, customEnv map[string]string) map[string]string {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return customEnv
	}
	m, err := h.Queries.GetLLMModelByModelID(ctx, modelID)
	if err != nil || !m.Enabled {
		return customEnv
	}
	cfg, err := h.Queries.GetLLMConfig(ctx, llmConfigID)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			slog.Warn("server llm: load config failed", "model_id", modelID, "error", err)
		}
		return customEnv
	}
	apiKey, keyErr := h.decryptLLMAPIKey(cfg)
	if keyErr != nil {
		slog.Warn("server llm: decrypt api key failed", "error", keyErr)
		return customEnv
	}
	baseURL := strings.TrimSpace(cfg.BaseUrl)
	if baseURL == "" && apiKey == "" {
		return customEnv
	}
	if customEnv == nil {
		customEnv = map[string]string{}
	}
	setIfEmpty := func(key, value string) {
		if value == "" {
			return
		}
		if strings.TrimSpace(customEnv[key]) == "" {
			customEnv[key] = value
		}
	}
	// OpenAI-compatible gateways are the common self-host shape; set both
	// OpenAI and Anthropic env names so Claude/Codex-style CLIs pick them up.
	setIfEmpty("OPENAI_BASE_URL", baseURL)
	setIfEmpty("OPENAI_API_BASE", baseURL)
	setIfEmpty("ANTHROPIC_BASE_URL", baseURL)
	setIfEmpty("OPENAI_API_KEY", apiKey)
	setIfEmpty("ANTHROPIC_API_KEY", apiKey)
	return customEnv
}
