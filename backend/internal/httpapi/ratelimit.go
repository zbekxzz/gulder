package httpapi

import (
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/httprate"
)

// Лимиты запросов в минуту (§18.5). 0 — лимит выключен.

func (s *Server) limiter(perMinute int, key httprate.KeyFunc) func(http.Handler) http.Handler {
	if perMinute <= 0 {
		return func(next http.Handler) http.Handler { return next }
	}
	return httprate.LimitBy(perMinute, time.Minute, key,
		httprate.WithLimitHandler(func(w http.ResponseWriter, r *http.Request) {
			writeError(w, r, &APIError{Status: http.StatusTooManyRequests, Code: "rate_limited", Message: "Слишком много запросов, попробуйте через минуту"})
		}))
}

// ClientIP — адрес клиента для лимитов. X-Forwarded-For учитывается, только если запрос пришёл
// от доверенного прокси (Caddy в сети compose): иначе клиент мог бы подделать заголовок и обойти
// лимит или «заблокировать» чужой IP. Берётся самый правый адрес, не принадлежащий доверенным прокси.
func ClientIP(r *http.Request, trusted []netip.Prefix) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	remote, err := netip.ParseAddr(host)
	if err != nil {
		return host
	}
	isTrusted := func(a netip.Addr) bool {
		for _, p := range trusted {
			if p.Contains(a.Unmap()) {
				return true
			}
		}
		return false
	}
	if !isTrusted(remote) {
		return remote.Unmap().String()
	}
	hops := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	for i := len(hops) - 1; i >= 0; i-- {
		a, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil {
			break
		}
		if !isTrusted(a) {
			return a.Unmap().String()
		}
	}
	return remote.Unmap().String()
}

// limitByIP — /auth/*: защита от перебора паролей.
func (s *Server) limitByIP(perMinute int) func(http.Handler) http.Handler {
	return s.limiter(perMinute, func(r *http.Request) (string, error) {
		return "ip:" + ClientIP(r, s.cfg.TrustedProxies), nil
	})
}

// limitByUser — дорогие эндпоинты (LLM, пересчёт рекомендаций) по пользователю.
func (s *Server) limitByUser(perMinute int) func(http.Handler) http.Handler {
	return s.limiter(perMinute, func(r *http.Request) (string, error) {
		return "user:" + strconv.FormatInt(principal(r).UserID, 10), nil
	})
}

// limitByToken — публичные приглашения (P1) по токену.
func (s *Server) limitByToken(perMinute int) func(http.Handler) http.Handler {
	return s.limiter(perMinute, func(r *http.Request) (string, error) {
		return "invite:" + chi.URLParam(r, "token"), nil
	})
}
