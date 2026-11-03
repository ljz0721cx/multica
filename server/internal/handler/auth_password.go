package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/multica-ai/multica/server/internal/auth"
	"github.com/multica-ai/multica/server/internal/logger"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

type PasswordLoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// LoginWithPassword authenticates via email + password for users that have a
// password set by a workspace admin. OTP / Google login remain available.
func (h *Handler) LoginWithPassword(w http.ResponseWriter, r *http.Request) {
	var req PasswordLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	password := req.Password
	if email == "" || password == "" {
		writeError(w, http.StatusBadRequest, "email and password are required")
		return
	}
	if auth.IsTemporarilyDisabledUserEmail(email) {
		writeError(w, http.StatusForbidden, auth.TemporarilyDisabledUserError)
		return
	}

	user, err := h.Queries.GetUserByEmail(r.Context(), email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusUnauthorized, "invalid email or password")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to look up user")
		return
	}

	stored, err := h.Queries.GetUserPassword(r.Context(), user.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusUnauthorized, "invalid email or password")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to look up password")
		return
	}

	ok, err := auth.CheckPassword(password, stored.PasswordHash)
	if err != nil || !ok {
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}

	tokenString, err := h.issueJWT(user)
	if err != nil {
		if errors.Is(err, auth.ErrTemporarilyDisabledUser) {
			writeError(w, http.StatusForbidden, auth.TemporarilyDisabledUserError)
			return
		}
		slog.Warn("password login failed", append(logger.RequestAttrs(r), "error", err, "email", email)...)
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	if err := auth.SetAuthCookies(w, tokenString); err != nil {
		slog.Warn("failed to set auth cookies", "error", err)
	}
	if h.CFSigner != nil {
		for _, cookie := range h.CFSigner.SignedCookies(time.Now().Add(auth.AuthTokenTTL())) {
			http.SetCookie(w, cookie)
		}
	}

	slog.Info("user logged in with password", append(logger.RequestAttrs(r), "user_id", uuidToString(user.ID), "email", user.Email)...)
	writeJSON(w, http.StatusOK, LoginResponse{
		Token: tokenString,
		User:  h.userToResponse(r.Context(), user),
	})
}

type ChangeOwnPasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// ChangeOwnPassword lets a signed-in user set or rotate their own login password.
func (h *Handler) ChangeOwnPassword(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	user, err := h.Queries.GetUser(r.Context(), parseUUID(userID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load user")
		return
	}

	var req ChangeOwnPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := auth.ValidatePasswordStrength(req.NewPassword); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	stored, err := h.Queries.GetUserPassword(r.Context(), user.ID)
	hasPassword := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusInternalServerError, "failed to look up password")
		return
	}
	if hasPassword {
		ok, checkErr := auth.CheckPassword(req.CurrentPassword, stored.PasswordHash)
		if checkErr != nil || !ok {
			writeError(w, http.StatusUnauthorized, "current password is incorrect")
			return
		}
	}

	hash, err := auth.HashPassword(req.NewPassword)
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

	writeJSON(w, http.StatusOK, h.userToResponse(r.Context(), user))
}
