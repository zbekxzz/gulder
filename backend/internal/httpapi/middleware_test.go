package httpapi

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRedactQuery(t *testing.T) {
	assert.Equal(t, "filter=nearby", RedactQuery("lat=43.25&lon=76.95&filter=nearby"))
	assert.Equal(t, "", RedactQuery(""))
	assert.Equal(t, "", RedactQuery("token=secret"))
}

// FR-L01, §18.4: геолокация и токены приглашений не попадают в логи.
func TestAccessLogOmitsGeolocation_FRL01(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(LogHandler{Handler: slog.NewJSONHandler(&buf, nil)}))
	t.Cleanup(func() { slog.SetDefault(prev) })

	h := requestIDMiddleware(accessLog(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))
	for _, target := range []string{
		"/api/v1/offers?filter=nearby&lat=43.238949&lon=76.889709&run_id=5",
		"/api/v1/public/invites/s3cr3t-token/answers",
	} {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, target, nil))
	}
	out := buf.String()
	assert.NotContains(t, out, "43.238949")
	assert.NotContains(t, out, "76.889709")
	assert.NotContains(t, out, "lat=")
	assert.NotContains(t, out, "s3cr3t-token")
	assert.Contains(t, out, "filter=nearby")
	assert.Contains(t, out, `"request_id"`)
}
