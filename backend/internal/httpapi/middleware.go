package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/zbekxzz/gulder/backend/internal/metrics"
)

type ctxKey int

const requestIDKey ctxKey = iota

// RequestID возвращает идентификатор запроса из контекста.
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

// LogHandler добавляет request_id в каждую запись slog (§18.4).
type LogHandler struct{ slog.Handler }

// Handle дописывает request_id из контекста.
func (h LogHandler) Handle(ctx context.Context, r slog.Record) error {
	if id := RequestID(ctx); id != "" {
		r.AddAttrs(slog.String("request_id", id))
	}
	return h.Handler.Handle(ctx, r) //nolint:wrapcheck // прозрачная обёртка
}

// WithAttrs сохраняет обёртку.
func (h LogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return LogHandler{h.Handler.WithAttrs(attrs)}
}

// WithGroup сохраняет обёртку.
func (h LogHandler) WithGroup(name string) slog.Handler { return LogHandler{h.Handler.WithGroup(name)} }

func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" || len(id) > 64 {
			var b [8]byte
			_, _ = rand.Read(b[:])
			id = hex.EncodeToString(b[:])
		}
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey, id)))
	})
}

// sensitiveQueryParams никогда не попадают в логи: геолокация (FR-L01) и токены (§18.4).
var sensitiveQueryParams = []string{"lat", "lon", "token", "access_token"}

// RedactQuery вырезает чувствительные параметры из query-строки.
func RedactQuery(raw string) string {
	if raw == "" {
		return ""
	}
	q, err := url.ParseQuery(raw)
	if err != nil {
		return "[unparseable]"
	}
	for _, k := range sensitiveQueryParams {
		q.Del(k)
	}
	return q.Encode()
}

// redactPath скрывает токен приглашения в пути /public/invites/{token}.
func redactPath(p string) string {
	const prefix = "/api/v1/public/invites/"
	if rest, ok := strings.CutPrefix(p, prefix); ok {
		_, tail, _ := strings.Cut(rest, "/")
		if tail != "" {
			return prefix + "[redacted]/" + tail
		}
		return prefix + "[redacted]"
	}
	return p
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// observe пишет латентность и код ответа в Prometheus по шаблону маршрута (без значений параметров).
func observe(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		route := "unmatched"
		if rc := chi.RouteContext(r.Context()); rc != nil && rc.RoutePattern() != "" {
			route = rc.RoutePattern()
		}
		metrics.HTTPDuration.WithLabelValues(r.Method, route, strconv.Itoa(rec.status)).Observe(time.Since(start).Seconds())
	})
}

// accessLog пишет одну запись на запрос без персональных данных и геолокации (§18.4).
func accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		slog.InfoContext(r.Context(), "http request",
			"method", r.Method,
			"path", redactPath(r.URL.Path),
			"query", RedactQuery(r.URL.RawQuery),
			"status", rec.status,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

func recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil && v != http.ErrAbortHandler { //nolint:errorlint // сравнение sentinel из recover
				slog.ErrorContext(r.Context(), "panic", "value", v)
				writeError(w, r, &APIError{Status: http.StatusInternalServerError, Code: "internal", Message: "Внутренняя ошибка"})
			}
		}()
		next.ServeHTTP(w, r)
	})
}
