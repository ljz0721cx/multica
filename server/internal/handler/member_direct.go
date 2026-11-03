package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/multica-ai/multica/server/internal/auth"
	"github.com/multica-ai/multica/server/internal/logger"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

// CreateMemberDirectRequest creates a workspace member immediately (no invite email).
type CreateMemberDirectRequest struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Remark   string `json:"remark"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

// SetMemberPasswordRequest sets or rotates a member's login password.
type SetMemberPasswordRequest struct {
	Password string `json:"password"`
}

// CreateMemberDirect adds a user to the workspace immediately and optionally sets a password.
// Admin/owner only. Unlike CreateInvitation, no email invite is required.
func (h *Handler) CreateMemberDirect(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromURL(r, "id")
	requester, ok := h.workspaceMember(w, r, workspaceID)
	if !ok {
		return
	}
	if requester.Role != "owner" && requester.Role != "admin" {
		writeError(w, http.StatusForbidden, "insufficient permissions")
		return
	}

	var req CreateMemberDirectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	if email == "" || !strings.Contains(email, "@") {
		writeError(w, http.StatusBadRequest, "valid email is required")
		return
	}
	role, valid := normalizeMemberRole(req.Role)
	if !valid {
		writeError(w, http.StatusBadRequest, "invalid member role")
		return
	}
	if role == "owner" && requester.Role != "owner" {
		writeError(w, http.StatusForbidden, "insufficient permissions")
		return
	}

	remark := strings.TrimSpace(req.Remark)
	if utf8RuneCount(remark) > 100 {
		writeError(w, http.StatusBadRequest, "remark must be at most 100 characters")
		return
	}

	password := req.Password
	if password != "" {
		if err := auth.ValidatePasswordStrength(password); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	displayName := strings.TrimSpace(req.Name)
	if displayName == "" {
		displayName = strings.TrimSpace(remark)
	}
	if displayName == "" {
		displayName = strings.Split(email, "@")[0]
	}

	user, err := h.Queries.GetUserByEmail(r.Context(), email)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusInternalServerError, "failed to look up user")
			return
		}
		user, err = h.Queries.CreateUser(r.Context(), db.CreateUserParams{
			Name:  displayName,
			Email: email,
		})
		if err != nil {
			slog.Warn("create member user failed", append(logger.RequestAttrs(r), "error", err)...)
			writeError(w, http.StatusInternalServerError, "failed to create user")
			return
		}
		if _, err := h.Queries.MarkUserOnboarded(r.Context(), user.ID); err != nil {
			slog.Warn("mark onboarded failed", append(logger.RequestAttrs(r), "error", err)...)
		}
	}

	if _, err := h.Queries.GetMemberByUserAndWorkspace(r.Context(), db.GetMemberByUserAndWorkspaceParams{
		UserID:      user.ID,
		WorkspaceID: requester.WorkspaceID,
	}); err == nil {
		writeError(w, http.StatusConflict, "user is already a member of this workspace")
		return
	} else if !errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusInternalServerError, "failed to check membership")
		return
	}

	member, err := h.Queries.CreateMember(r.Context(), db.CreateMemberParams{
		WorkspaceID: requester.WorkspaceID,
		UserID:      user.ID,
		Role:        role,
		Remark:      remark,
	})
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "user is already a member of this workspace")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create membership")
		return
	}

	hasPassword := false
	if password != "" {
		hash, err := auth.HashPassword(password)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if _, err := h.Queries.UpsertUserPassword(r.Context(), db.UpsertUserPasswordParams{
			UserID:       user.ID,
			PasswordHash: hash,
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to set password")
			return
		}
		hasPassword = true
	}

	userID := requestUserID(r)
	resp := h.memberWithUserResponse(r.Context(), member, user)
	resp.HasPassword = hasPassword
	h.publish(protocol.EventMemberAdded, uuidToString(requester.WorkspaceID), "member", userID, map[string]any{
		"member": resp,
	})
	writeJSON(w, http.StatusCreated, resp)
}

// SetMemberPassword lets a workspace admin set a member's password login.
func (h *Handler) SetMemberPassword(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromURL(r, "id")
	requester, ok := h.workspaceMember(w, r, workspaceID)
	if !ok {
		return
	}
	if requester.Role != "owner" && requester.Role != "admin" {
		writeError(w, http.StatusForbidden, "insufficient permissions")
		return
	}

	memberID := chi.URLParam(r, "memberId")
	memberUUID, ok := parseUUIDOrBadRequest(w, memberID, "member id")
	if !ok {
		return
	}
	target, err := h.Queries.GetMember(r.Context(), memberUUID)
	if err != nil || uuidToString(target.WorkspaceID) != uuidToString(requester.WorkspaceID) {
		writeError(w, http.StatusNotFound, "member not found")
		return
	}

	var req SetMemberPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := auth.ValidatePasswordStrength(req.Password); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := h.Queries.UpsertUserPassword(r.Context(), db.UpsertUserPasswordParams{
		UserID:       target.UserID,
		PasswordHash: hash,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to set password")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func utf8RuneCount(s string) int {
	return len([]rune(s))
}
