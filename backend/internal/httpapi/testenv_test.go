package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/zbekxzz/gulder/backend/internal/auth"
	"github.com/zbekxzz/gulder/backend/internal/catalog"
	"github.com/zbekxzz/gulder/backend/internal/config"
	"github.com/zbekxzz/gulder/backend/internal/httpapi"
	"github.com/zbekxzz/gulder/backend/internal/store"
	"github.com/zbekxzz/gulder/backend/internal/testdb"
)

// env — API на чистой базе с сидом и dev-пользователями.
type env struct {
	t     *testing.T
	srv   *httptest.Server
	store *store.Store
	now   time.Time
}

func newEnv(t *testing.T, opts ...httpapi.Option) *env {
	t.Helper()
	st, _ := testdb.New(t)
	ctx := context.Background()
	require.NoError(t, st.InTx(ctx, func(tx pgx.Tx) error {
		if _, err := catalog.Seed(ctx, tx, "../../../data"); err != nil {
			return err
		}
		_, err := auth.SeedDevUsers(ctx, tx)
		return err
	}))
	cfg := config.Config{
		Env: "test", JWTSecret: []byte(strings.Repeat("k", 32)),
		AccessTokenTTL: time.Hour, RefreshTokenTTL: 24 * time.Hour,
		CORSOrigins: []string{"http://localhost:5173"},
	}
	// фиксированное «сейчас»: понедельник 20.10.2026 12:00 по Алматы — скидки сида действуют
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.FixedZone("ALMT", 5*3600))
	opts = append([]httpapi.Option{httpapi.WithClock(func() time.Time { return now })}, opts...)
	srv := httptest.NewServer(httpapi.New(cfg, st, opts...).Router())
	t.Cleanup(srv.Close)
	return &env{t: t, srv: srv, store: st, now: now}
}

type resp struct {
	Status int
	Body   []byte
}

func (r resp) JSON(t *testing.T) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal(r.Body, &m), string(r.Body))
	return m
}

func (r resp) Decode(t *testing.T, dst any) {
	t.Helper()
	require.NoError(t, json.Unmarshal(r.Body, dst), string(r.Body))
}

func (e *env) do(method, path, token string, body any) resp {
	e.t.Helper()
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(e.t, err)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, e.srv.URL+path, rd)
	require.NoError(e.t, err)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	require.NoError(e.t, err)
	defer func() { _ = res.Body.Close() }()
	b, _ := io.ReadAll(res.Body)
	return resp{Status: res.StatusCode, Body: b}
}

func (e *env) upload(path, token, csv string) resp {
	e.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", "import.csv")
	require.NoError(e.t, err)
	_, _ = fw.Write([]byte(csv))
	require.NoError(e.t, mw.Close())
	req, err := http.NewRequest(http.MethodPost, e.srv.URL+path, &buf)
	require.NoError(e.t, err)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	require.NoError(e.t, err)
	defer func() { _ = res.Body.Close() }()
	b, _ := io.ReadAll(res.Body)
	return resp{Status: res.StatusCode, Body: b}
}

func (e *env) login(email string) string {
	e.t.Helper()
	r := e.do(http.MethodPost, "/api/v1/auth/login", "", map[string]string{"email": email, "password": auth.DevPassword})
	require.Equal(e.t, http.StatusOK, r.Status, string(r.Body))
	return r.JSON(e.t)["access_token"].(string)
}

// attr — поле профиля для запроса.
func attr(key string, value any, conf string) map[string]any {
	return map[string]any{"key": key, "value": value, "source": "giver", "confidence": conf}
}

func (e *env) createRecipient(token, nickname string, attrs ...map[string]any) int64 {
	e.t.Helper()
	if attrs == nil {
		attrs = []map[string]any{}
	}
	r := e.do(http.MethodPost, "/api/v1/recipients", token, map[string]any{"nickname": nickname, "attributes": attrs})
	require.Equal(e.t, http.StatusCreated, r.Status, string(r.Body))
	return int64(r.JSON(e.t)["id"].(float64))
}

func (e *env) createOccasion(token string, recipientID int64, budgetMax int, extra map[string]any) int64 {
	e.t.Helper()
	body := map[string]any{"recipient_id": recipientID, "type": "birthday", "budget_min": 0, "budget_max": budgetMax, "date": "2026-10-25"}
	for k, v := range extra {
		body[k] = v
	}
	r := e.do(http.MethodPost, "/api/v1/occasions", token, body)
	require.Equal(e.t, http.StatusCreated, r.Status, string(r.Body))
	return int64(r.JSON(e.t)["id"].(float64))
}
