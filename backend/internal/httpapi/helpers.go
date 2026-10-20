package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"

	"github.com/zbekxzz/gulder/backend/internal/auth"
)

var validate = validator.New(validator.WithRequiredStructEnabled())

// validateStruct проверяет теги `validate` и возвращает 422 с полями.
func validateStruct(v any) error {
	err := validate.Struct(v)
	if err == nil {
		return nil
	}
	var verrs validator.ValidationErrors
	if !errors.As(err, &verrs) {
		return errBadRequest("Некорректный запрос", nil)
	}
	details := map[string]any{}
	for _, fe := range verrs {
		details[strings.ToLower(fe.Field())] = fe.Tag()
	}
	return errValidation(details)
}

func pathID(r *http.Request, name string) (int64, error) {
	id, err := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	if err != nil || id <= 0 {
		return 0, errNotFound
	}
	return id, nil
}

func principal(r *http.Request) auth.Principal {
	p, _ := auth.FromContext(r.Context())
	return p
}

// floristShop возвращает магазин флориста; флорист без магазина ничего не меняет.
func floristShop(r *http.Request) (int64, error) {
	p := principal(r)
	if p.ShopID == nil {
		return 0, &APIError{Status: http.StatusForbidden, Code: "no_shop", Message: "Флорист не привязан к магазину"}
	}
	return *p.ShopID, nil
}

// authenticate проверяет access-токен: нет или неверный → 401.
func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok := auth.BearerToken(r)
		if tok == "" {
			writeError(w, r, errUnauthorized)
			return
		}
		p, err := s.issuer.Parse(tok, auth.TokenAccess)
		if err != nil {
			writeError(w, r, errUnauthorized)
			return
		}
		next.ServeHTTP(w, r.WithContext(auth.WithPrincipal(r.Context(), p)))
	})
}

// requireRole пропускает только указанные роли: иначе 403.
func requireRole(roles ...auth.Role) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p := principal(r)
			for _, role := range roles {
				if p.Role == role {
					next.ServeHTTP(w, r)
					return
				}
			}
			writeError(w, r, errForbidden)
		})
	}
}
