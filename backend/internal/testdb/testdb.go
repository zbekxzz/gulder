// Package testdb поднимает чистую PostgreSQL 16 для интеграционных тестов.
//
// Источник БД:
//   - TEST_DATABASE_URL — уже запущенный сервер (локально без Docker); для каждого вызова
//     создаётся отдельная база, которая удаляется после теста;
//   - иначе testcontainers-go с образом postgres:16-alpine (CI);
//   - если Docker недоступен — тест пропускается (t.Skip), а не падает.
package testdb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/zbekxzz/gulder/backend/internal/store"
)

var (
	containerOnce sync.Once
	containerURL  string
	containerErr  error
)

func serverURL(t *testing.T) string {
	t.Helper()
	if u := os.Getenv("TEST_DATABASE_URL"); u != "" {
		return u
	}
	containerOnce.Do(func() {
		defer func() {
			if r := recover(); r != nil {
				containerErr = fmt.Errorf("docker unavailable: %v", r)
			}
		}()
		ctx := context.Background()
		c, err := postgres.Run(ctx, "postgres:16-alpine",
			postgres.WithDatabase("postgres"), postgres.WithUsername("postgres"), postgres.WithPassword("postgres"),
			testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(60*time.Second)))
		if err != nil {
			containerErr = err
			return
		}
		containerURL, containerErr = c.ConnectionString(ctx, "sslmode=disable")
	})
	if containerErr != nil {
		t.Skipf("integration test skipped: no TEST_DATABASE_URL and %v", containerErr)
	}
	return containerURL
}

// New создаёт пустую базу с применёнными миграциями и возвращает Store и её URL.
func New(t *testing.T) (*store.Store, string) {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test skipped in -short mode")
	}
	ctx := context.Background()
	base := serverURL(t)

	var b [6]byte
	_, _ = rand.Read(b[:])
	name := "test_" + hex.EncodeToString(b[:])

	admin, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Fatalf("connect admin: %v", err)
	}
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatalf("create database: %v", err)
	}
	_ = admin.Close(ctx)

	u, err := url.Parse(base)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	u.Path = "/" + name
	dbURL := u.String()

	if err := store.Migrate(ctx, dbURL, "up"); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	st, err := store.Open(ctx, dbURL)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		st.Close()
		admin, err := pgx.Connect(context.Background(), base)
		if err == nil {
			_, _ = admin.Exec(context.Background(), "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
			_ = admin.Close(context.Background())
		}
	})
	return st, dbURL
}
