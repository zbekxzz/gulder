package httpapi_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zbekxzz/gulder/backend/internal/auth"
	"github.com/zbekxzz/gulder/backend/internal/config"
	"github.com/zbekxzz/gulder/backend/internal/metrics"
)

// §18.5: /auth/* ограничен по IP, дорогие эндпоинты — по пользователю.
func TestRateLimits(t *testing.T) {
	e := newEnvWithConfig(t, func(c *config.Config) { c.RateLimitAuth = 3; c.RateLimitHeavy = 2 })
	body := map[string]string{"email": "giver@gulder.local", "password": auth.DevPassword}
	for i := 0; i < 3; i++ {
		require.Equal(t, http.StatusOK, e.do(http.MethodPost, "/api/v1/auth/login", "", body).Status)
	}
	r := e.do(http.MethodPost, "/api/v1/auth/login", "", body)
	assert.Equal(t, http.StatusTooManyRequests, r.Status)
	assert.Equal(t, "rate_limited", r.JSON(t)["error"].(map[string]any)["code"])

	token := ""
	{ // лимит на /recommendations проверяем в отдельной среде без лимита на вход
		e2 := newEnvWithConfig(t, func(c *config.Config) { c.RateLimitHeavy = 2 })
		token = e2.login("giver@gulder.local")
		id := e2.createRecipient(token, "X")
		occ := e2.createOccasion(token, id, 20000, nil)
		for i := 0; i < 2; i++ {
			require.Equal(t, http.StatusCreated, e2.do(http.MethodPost, "/api/v1/recommendations", token, map[string]any{"occasion_id": occ}).Status)
		}
		assert.Equal(t, http.StatusTooManyRequests, e2.do(http.MethodPost, "/api/v1/recommendations", token, map[string]any{"occasion_id": occ}).Status)
		// другой пользователь не затронут
		other := e2.login("giver2@gulder.local")
		id2 := e2.createRecipient(other, "Y")
		occ2 := e2.createOccasion(other, id2, 20000, nil)
		assert.Equal(t, http.StatusCreated, e2.do(http.MethodPost, "/api/v1/recommendations", other, map[string]any{"occasion_id": occ2}).Status)
	}
	assert.NotEmpty(t, token)
}

// §18.4: метрики — латентность по шаблону маршрута и rules_block_total по кодам, без персональных данных.
func TestMetrics(t *testing.T) {
	e := newEnv(t)
	giver := e.login("giver@gulder.local")
	id := e.createRecipient(giver, "Кошатница Айгерим", attr("pets", []string{"cat"}, "know"))
	e.recommend(giver, e.createOccasion(giver, id, 40000, nil))
	e.do(http.MethodGet, "/api/v1/recipients/"+itoa(id), giver, nil)

	rec := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	out, _ := io.ReadAll(rec.Body)
	text := string(out)
	assert.Contains(t, text, `rules_block_total{code="PET_TOXIC"}`)
	assert.Contains(t, text, `route="/api/v1/recommendations"`)
	assert.Contains(t, text, `route="/api/v1/recipients/{id}"`)
	assert.NotContains(t, text, "/api/v1/recipients/"+itoa(id)+`"`, "значения параметров не попадают в метки")
	assert.NotContains(t, text, "Айгерим")
	assert.False(t, strings.Contains(text, "giver@"), "email не попадает в метрики")
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
