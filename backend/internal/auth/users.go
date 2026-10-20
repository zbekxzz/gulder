package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/zbekxzz/gulder/backend/internal/store"
)

// User — учётная запись.
type User struct {
	ID          int64  `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Role        Role   `json:"role"`
	ShopID      *int64 `json:"shop_id"`
}

// Principal возвращает субъект для токена.
func (u User) Principal() Principal { return Principal{UserID: u.ID, Role: u.Role, ShopID: u.ShopID} }

// Ошибки учётных записей.
var (
	ErrEmailTaken         = errors.New("email already registered")
	ErrInvalidCredentials = errors.New("invalid credentials")
)

// NormalizeEmail приводит email к каноническому виду.
func NormalizeEmail(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// CreateUser создаёт пользователя с паролем.
func CreateUser(ctx context.Context, q store.Querier, email, password, name string, role Role, shopID *int64) (User, error) {
	hash, err := HashPassword(password)
	if err != nil {
		return User{}, err
	}
	u := User{Email: NormalizeEmail(email), DisplayName: name, Role: role, ShopID: shopID}
	err = q.QueryRow(ctx, `INSERT INTO users (email, display_name, role, shop_id, password_hash)
		VALUES ($1, NULLIF($2,''), $3, $4, $5) RETURNING id`, u.Email, name, string(role), shopID, hash).Scan(&u.ID)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return User{}, ErrEmailTaken
	}
	if err != nil {
		return User{}, fmt.Errorf("insert user: %w", err)
	}
	return u, nil
}

// Authenticate проверяет email и пароль. Ошибка одинакова для «нет пользователя» и «неверный пароль».
func Authenticate(ctx context.Context, q store.Querier, email, password string) (User, error) {
	var u User
	var hash *string
	var name *string
	err := q.QueryRow(ctx, `SELECT id, email, display_name, role, shop_id, password_hash FROM users WHERE email = $1`,
		NormalizeEmail(email)).Scan(&u.ID, &u.Email, &name, &u.Role, &u.ShopID, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		// тратим время на хэш, чтобы не раскрывать существование email по таймингу
		_, _ = VerifyPassword(password, dummyHash)
		return User{}, ErrInvalidCredentials
	}
	if err != nil {
		return User{}, fmt.Errorf("load user: %w", err)
	}
	if hash == nil {
		return User{}, ErrInvalidCredentials
	}
	ok, err := VerifyPassword(password, *hash)
	if err != nil || !ok {
		return User{}, ErrInvalidCredentials
	}
	if name != nil {
		u.DisplayName = *name
	}
	return u, nil
}

// GetUser загружает пользователя по id.
func GetUser(ctx context.Context, q store.Querier, id int64) (User, error) {
	var u User
	var name *string
	err := q.QueryRow(ctx, `SELECT id, email, display_name, role, shop_id FROM users WHERE id = $1`, id).
		Scan(&u.ID, &u.Email, &name, &u.Role, &u.ShopID)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, store.ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("load user: %w", err)
	}
	if name != nil {
		u.DisplayName = *name
	}
	return u, nil
}

var dummyHash = func() string {
	h, _ := HashPassword("dummy-password-for-timing")
	return h
}()
