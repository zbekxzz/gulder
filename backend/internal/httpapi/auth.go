package httpapi

import (
	"errors"
	"net/http"

	"github.com/zbekxzz/gulder/backend/internal/auth"
)

type registerRequest struct {
	Email       string `json:"email" validate:"required,email,max=254"`
	Password    string `json:"password" validate:"required,min=8,max=128"`
	DisplayName string `json:"display_name" validate:"max=100"`
}

type loginRequest struct {
	Email    string `json:"email" validate:"required"`
	Password string `json:"password" validate:"required"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

type authResponse struct {
	auth.Tokens
	User auth.User `json:"user"`
}

// register — самостоятельная регистрация доступна только дарителю; флористов и менеджеров заводит администратор.
func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	if err := validateStruct(req); err != nil {
		writeError(w, r, err)
		return
	}
	u, err := auth.CreateUser(r.Context(), s.store.Pool, req.Email, req.Password, req.DisplayName, auth.RoleGiver, nil)
	if errors.Is(err, auth.ErrEmailTaken) {
		writeError(w, r, errConflict("email_taken", "Этот email уже зарегистрирован", nil))
		return
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	s.respondTokens(w, r, http.StatusCreated, u)
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	if err := validateStruct(req); err != nil {
		writeError(w, r, err)
		return
	}
	u, err := auth.Authenticate(r.Context(), s.store.Pool, req.Email, req.Password)
	if errors.Is(err, auth.ErrInvalidCredentials) {
		writeError(w, r, &APIError{Status: http.StatusUnauthorized, Code: "invalid_credentials", Message: "Неверный email или пароль"})
		return
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	s.respondTokens(w, r, http.StatusOK, u)
}

func (s *Server) refresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	p, err := s.issuer.Parse(req.RefreshToken, auth.TokenRefresh)
	if err != nil {
		writeError(w, r, errUnauthorized)
		return
	}
	// роль и магазин перечитываются из БД: изменения прав действуют без повторного входа
	u, err := auth.GetUser(r.Context(), s.store.Pool, p.UserID)
	if err != nil {
		writeError(w, r, errUnauthorized)
		return
	}
	s.respondTokens(w, r, http.StatusOK, u)
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	u, err := auth.GetUser(r.Context(), s.store.Pool, principal(r).UserID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (s *Server) respondTokens(w http.ResponseWriter, r *http.Request, status int, u auth.User) {
	toks, err := s.issuer.Issue(u.Principal())
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, status, authResponse{Tokens: toks, User: u})
}
