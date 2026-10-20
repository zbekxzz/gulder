package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/zbekxzz/gulder/backend/internal/store"
)

// APIError — ошибка в формате §14: {"error":{"code","message","details"}}.
type APIError struct {
	Status  int            `json:"-"`
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

func (e *APIError) Error() string { return e.Code + ": " + e.Message }

var (
	errNotFound       = &APIError{Status: http.StatusNotFound, Code: "not_found", Message: "Не найдено"}
	errNotImplemented = &APIError{Status: http.StatusNotImplemented, Code: "not_implemented", Message: "Функция появится в релизе 2.0"}
)

// writeJSON пишет ответ.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

// writeError маппит ошибку в HTTP-код §14. Внутренние детали не уходят клиенту.
func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *APIError
	switch {
	case errors.As(err, &apiErr):
	case errors.Is(err, store.ErrNotFound):
		apiErr = errNotFound
	default:
		slog.ErrorContext(r.Context(), "request failed", "err", err)
		apiErr = &APIError{Status: http.StatusInternalServerError, Code: "internal", Message: "Внутренняя ошибка"}
	}
	writeJSON(w, apiErr.Status, map[string]any{"error": apiErr})
}
