package handler

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
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
	writeJSON(w, http.StatusOK, map[string]any{
		"can_manage": isModelAdmin(user),
	})
}
