package httpapi

import (
	"net/http"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/zbekxzz/gulder/backend/internal/config"
)

// Каждый маршрут /api/v1 описан в backend/api/openapi.yaml, и наоборот: TS-типы фронта
// генерируются из контракта, поэтому расхождение — это ошибка (§3).
func TestOpenAPICoversRoutes(t *testing.T) {
	raw, err := os.ReadFile("../../api/openapi.yaml")
	require.NoError(t, err)
	var spec struct {
		Paths map[string]map[string]any `yaml:"paths"`
	}
	require.NoError(t, yaml.Unmarshal(raw, &spec))

	documented := map[string]bool{}
	for path, ops := range spec.Paths {
		for method := range ops {
			if method == "parameters" {
				continue
			}
			documented[strings.ToUpper(method)+" "+path] = true
		}
	}

	srv := New(config.Config{JWTSecret: []byte(strings.Repeat("k", 32))}, nil)
	router, ok := srv.Router().(chi.Routes)
	require.True(t, ok)
	implemented := map[string]bool{}
	require.NoError(t, chi.Walk(router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if rest, ok := strings.CutPrefix(route, "/api/v1"); ok && method != http.MethodOptions && method != http.MethodHead {
			implemented[method+" "+strings.TrimSuffix(rest, "/")] = true
		}
		return nil
	}))

	var missing, stale []string
	for r := range implemented {
		if !documented[r] {
			missing = append(missing, r)
		}
	}
	for r := range documented {
		if !implemented[r] {
			stale = append(stale, r)
		}
	}
	sort.Strings(missing)
	sort.Strings(stale)
	require.Empty(t, missing, "маршруты без описания в openapi.yaml")
	require.Empty(t, stale, "описаны в openapi.yaml, но не реализованы")
}
