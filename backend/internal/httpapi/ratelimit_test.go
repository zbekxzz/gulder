package httpapi

import (
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
)

// X-Forwarded-For учитывается только от доверенного прокси — иначе лимит обходится подделкой заголовка.
func TestClientIP(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("172.16.0.0/12")}
	req := func(remote, xff string) string {
		r := httptest.NewRequest("POST", "/api/v1/auth/login", nil)
		r.RemoteAddr = remote
		if xff != "" {
			r.Header.Set("X-Forwarded-For", xff)
		}
		return ClientIP(r, trusted)
	}
	assert.Equal(t, "203.0.113.7", req("203.0.113.7:5555", "1.2.3.4"), "клиент напрямую — заголовок игнорируется")
	assert.Equal(t, "198.51.100.9", req("172.18.0.5:443", "1.2.3.4, 198.51.100.9"), "через Caddy — правый адрес, добавленный прокси")
	assert.Equal(t, "172.18.0.5", req("172.18.0.5:443", "мусор"), "битый заголовок — адрес прокси")
	assert.Equal(t, "203.0.113.7", req("203.0.113.7:1", ""), "без прокси")
	assert.Equal(t, "192.0.2.1", ClientIP(httptest.NewRequest("GET", "/", nil), nil), "без списка доверенных прокси — RemoteAddr")
}
