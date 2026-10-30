package http

import (
	"errors"
	"net/http"

	"orchestrator/internal/application/authentication"
	"orchestrator/internal/domain/identity"
)

// userView is the public account DTO; the password hash never leaves the
// server (UC-00 BR-02/BR-06).
type userView struct {
	ID       string                 `json:"id"`
	Username string                 `json:"username"`
	Role     identity.Role          `json:"role"`
	Status   identity.AccountStatus `json:"status"`
}

func userViewOf(a identity.UserAccount) userView {
	return userView{ID: a.ID, Username: a.Username, Role: a.Role, Status: a.Status}
}

type changePasswordRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
	ConfirmPassword string `json:"confirmPassword"`
}

// handleChangePassword implements UC-00 BR-08: the signed-in user proves the
// current password, chooses a new one and every session of the account ends,
// including this one; the cookie is cleared so the client signs in again.
func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	who, ok := s.sessionIdentity(r)
	if !ok {
		writeCode(w, codeUnauthorized, "")
		return
	}
	var req changePasswordRequest
	if err := decodeSingleObject(r, &req); err != nil {
		writeCode(w, codeInvalidRequest, "")
		return
	}
	if req.CurrentPassword == "" {
		writeCode(w, codeInvalidRequest, "currentPassword")
		return
	}
	if req.NewPassword == "" {
		writeCode(w, codeInvalidRequest, "newPassword")
		return
	}
	if req.ConfirmPassword != req.NewPassword {
		writeCode(w, codeInvalidRequest, "confirmPassword")
		return
	}
	err := s.auth.ChangePassword(r.Context(), who.UserID, req.CurrentPassword, req.NewPassword)
	switch {
	case err == nil:
	case errors.Is(err, authentication.ErrInvalidCredentials):
		writeCode(w, codeInvalidCredentials, "")
		return
	case errors.Is(err, authentication.ErrInvalidPassword):
		writeCode(w, codeInvalidRequest, "newPassword")
		return
	default:
		s.writeError(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, MaxAge: -1})
	w.WriteHeader(http.StatusNoContent)
}

// handleListUsers implements UC-00 BR-09: only an Admin lists the accounts of
// their Organization; the role comes from the session, never from the client.
func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	who, ok := s.sessionIdentity(r)
	if !ok {
		writeCode(w, codeUnauthorized, "")
		return
	}
	accounts, err := s.auth.ListUsers(r.Context(), who)
	if err != nil {
		if errors.Is(err, authentication.ErrForbidden) {
			writeCode(w, codeForbidden, "")
			return
		}
		s.writeError(w, err)
		return
	}
	out := make([]userView, 0, len(accounts))
	for _, a := range accounts {
		out = append(out, userViewOf(a))
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": out})
}

type createUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

// handleCreateUser implements UC-00 BR-09: an Admin creates an account with a
// username, password and one of the three roles. Usernames are unique; invalid
// input is refused with the offending field.
func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	who, ok := s.sessionIdentity(r)
	if !ok {
		writeCode(w, codeUnauthorized, "")
		return
	}
	var req createUserRequest
	if err := decodeSingleObject(r, &req); err != nil {
		writeCode(w, codeInvalidRequest, "")
		return
	}
	account, err := s.auth.CreateUser(r.Context(), who, req.Username, req.Password, identity.Role(req.Role))
	switch {
	case err == nil:
	case errors.Is(err, authentication.ErrForbidden):
		writeCode(w, codeForbidden, "")
		return
	case errors.Is(err, authentication.ErrUsernameTaken):
		writeCode(w, codeDuplicate, "username")
		return
	case errors.Is(err, authentication.ErrInvalidUsername):
		writeCode(w, codeInvalidRequest, "username")
		return
	case errors.Is(err, authentication.ErrInvalidRole):
		writeCode(w, codeInvalidRequest, "role")
		return
	case errors.Is(err, authentication.ErrInvalidPassword):
		writeCode(w, codeInvalidRequest, "password")
		return
	default:
		s.writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"user": userViewOf(account)})
}

type resetPasswordRequest struct {
	NewPassword     string `json:"newPassword"`
	ConfirmPassword string `json:"confirmPassword"`
}

// handleResetUserPassword implements UC-00 BR-11: an Admin sets a new password
// for an account of the same Organization without knowing the current one, and
// every session of that account is revoked so it must sign in again. The role
// comes from the session, never from the client.
func (s *Server) handleResetUserPassword(w http.ResponseWriter, r *http.Request) {
	who, ok := s.sessionIdentity(r)
	if !ok {
		writeCode(w, codeUnauthorized, "")
		return
	}
	var req resetPasswordRequest
	if err := decodeSingleObject(r, &req); err != nil {
		writeCode(w, codeInvalidRequest, "")
		return
	}
	if req.NewPassword == "" {
		writeCode(w, codeInvalidRequest, "newPassword")
		return
	}
	if req.ConfirmPassword != req.NewPassword {
		writeCode(w, codeInvalidRequest, "confirmPassword")
		return
	}
	err := s.auth.ResetUserPassword(r.Context(), who, r.PathValue("id"), req.NewPassword)
	switch {
	case err == nil:
	case errors.Is(err, authentication.ErrForbidden):
		writeCode(w, codeForbidden, "")
		return
	case errors.Is(err, authentication.ErrInvalidPassword):
		writeCode(w, codeInvalidRequest, "newPassword")
		return
	case errors.Is(err, authentication.ErrUserNotFound):
		writeCode(w, codeNotFound, "")
		return
	default:
		s.writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
