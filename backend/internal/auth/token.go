package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Role — роль пользователя.
type Role string

// Роли (§2).
const (
	RoleGiver   Role = "giver"
	RoleFlorist Role = "florist"
	RoleManager Role = "manager"
)

// Типы токенов.
const (
	TokenAccess  = "access"
	TokenRefresh = "refresh"
)

// Principal — аутентифицированный пользователь запроса.
type Principal struct {
	UserID int64
	Role   Role
	ShopID *int64 // для флориста
}

// Claims — полезная нагрузка JWT.
type Claims struct {
	Role   Role   `json:"role"`
	ShopID *int64 `json:"shop_id,omitempty"`
	Type   string `json:"typ"`
	jwt.RegisteredClaims
}

// Tokens — пара токенов.
type Tokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

// Issuer выпускает и проверяет токены (HS256).
type Issuer struct {
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
	now        func() time.Time
}

// NewIssuer создаёт Issuer.
func NewIssuer(secret []byte, accessTTL, refreshTTL time.Duration) *Issuer {
	return &Issuer{secret: secret, accessTTL: accessTTL, refreshTTL: refreshTTL, now: time.Now}
}

// Issue выпускает access и refresh токены.
func (i *Issuer) Issue(p Principal) (Tokens, error) {
	access, err := i.sign(p, TokenAccess, i.accessTTL)
	if err != nil {
		return Tokens{}, err
	}
	refresh, err := i.sign(p, TokenRefresh, i.refreshTTL)
	if err != nil {
		return Tokens{}, err
	}
	return Tokens{AccessToken: access, RefreshToken: refresh, ExpiresIn: int(i.accessTTL.Seconds())}, nil
}

func (i *Issuer) sign(p Principal, typ string, ttl time.Duration) (string, error) {
	now := i.now()
	c := Claims{
		Role: p.Role, ShopID: p.ShopID, Type: typ,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatInt(p.UserID, 10),
			Issuer:    "gulder",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(i.secret)
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}
	return s, nil
}

// ErrInvalidToken — токен не прошёл проверку.
var ErrInvalidToken = errors.New("invalid token")

// Parse проверяет подпись, срок и тип токена.
func (i *Issuer) Parse(token, wantType string) (Principal, error) {
	var c Claims
	_, err := jwt.ParseWithClaims(token, &c, func(*jwt.Token) (any, error) { return i.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer("gulder"),
		jwt.WithTimeFunc(i.now),
		jwt.WithExpirationRequired())
	if err != nil || c.Type != wantType {
		return Principal{}, ErrInvalidToken
	}
	id, err := strconv.ParseInt(c.Subject, 10, 64)
	if err != nil {
		return Principal{}, ErrInvalidToken
	}
	switch c.Role {
	case RoleGiver, RoleFlorist, RoleManager:
	default:
		return Principal{}, ErrInvalidToken
	}
	return Principal{UserID: id, Role: c.Role, ShopID: c.ShopID}, nil
}

type ctxKey struct{}

// WithPrincipal кладёт пользователя в контекст.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, ctxKey{}, p)
}

// FromContext возвращает пользователя запроса.
func FromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(ctxKey{}).(Principal)
	return p, ok
}

// BearerToken извлекает токен из заголовка Authorization.
func BearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if t, ok := strings.CutPrefix(h, "Bearer "); ok {
		return strings.TrimSpace(t)
	}
	return ""
}
