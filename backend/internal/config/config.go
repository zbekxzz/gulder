// Package config читает настройки сервиса из переменных окружения (§3).
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config — настройки процесса. Секреты приходят только из env (§18.5).
type Config struct {
	Env             string // dev | test | prod
	HTTPAddr        string
	DatabaseURL     string
	LogLevel        slog.Level
	JWTSecret       []byte
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
	CORSOrigins     []string
	AnthropicAPIKey string
	LLMModel        string
	LLMTimeout      time.Duration
	DataDir         string
	RankingConfig   string         // путь к ranking.yaml; пусто = встроенный
	MetricsAddr     string         // отдельный адрес для /metrics; пусто — не поднимать
	RateLimitAuth   int            // запросов в минуту на IP для /auth/*
	RateLimitHeavy  int            // запросов в минуту на пользователя для /recommendations и /gift-sets
	RateLimitInvite int            // запросов в минуту на токен приглашения
	TrustedProxies  []netip.Prefix // от них принимается X-Forwarded-For (Caddy)
}

// IsDev сообщает, что сервис запущен в режиме разработки.
func (c Config) IsDev() bool { return c.Env == "dev" || c.Env == "test" }

// Load читает конфигурацию из env.
func Load() (Config, error) {
	c := Config{
		Env:             get("APP_ENV", "dev"),
		HTTPAddr:        get("HTTP_ADDR", ":8080"),
		DatabaseURL:     os.Getenv("DATABASE_URL"),
		JWTSecret:       []byte(os.Getenv("JWT_SECRET")),
		CORSOrigins:     splitList(get("CORS_ORIGINS", "http://localhost:5173")),
		AnthropicAPIKey: os.Getenv("ANTHROPIC_API_KEY"),
		LLMModel:        get("LLM_MODEL", "claude-opus-5-5"),
		DataDir:         get("DATA_DIR", "../data"),
		RankingConfig:   os.Getenv("RANKING_CONFIG"),
		MetricsAddr:     get("METRICS_ADDR", ":9090"),
		RateLimitAuth:   Int("RATE_LIMIT_AUTH", 30),
		RateLimitHeavy:  Int("RATE_LIMIT_HEAVY", 30),
		RateLimitInvite: Int("RATE_LIMIT_INVITE", 10),
	}
	var err error
	if c.LogLevel, err = parseLevel(get("LOG_LEVEL", "info")); err != nil {
		return c, err
	}
	if c.AccessTokenTTL, err = time.ParseDuration(get("ACCESS_TOKEN_TTL", "15m")); err != nil {
		return c, fmt.Errorf("ACCESS_TOKEN_TTL: %w", err)
	}
	if c.RefreshTokenTTL, err = time.ParseDuration(get("REFRESH_TOKEN_TTL", "720h")); err != nil {
		return c, fmt.Errorf("REFRESH_TOKEN_TTL: %w", err)
	}
	if c.LLMTimeout, err = time.ParseDuration(get("LLM_TIMEOUT", "20s")); err != nil {
		return c, fmt.Errorf("LLM_TIMEOUT: %w", err)
	}
	for _, p := range splitList(get("TRUSTED_PROXIES", "")) {
		prefix, err := netip.ParsePrefix(p)
		if err != nil {
			return c, fmt.Errorf("TRUSTED_PROXIES: %w", err)
		}
		c.TrustedProxies = append(c.TrustedProxies, prefix)
	}
	if c.DatabaseURL == "" {
		return c, errors.New("DATABASE_URL is required")
	}
	if len(c.JWTSecret) < 32 {
		if !c.IsDev() {
			return c, errors.New("JWT_SECRET must be at least 32 bytes outside dev")
		}
		c.JWTSecret = []byte("dev-only-insecure-jwt-secret-change-me!!")
	}
	return c, nil
}

func get(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseLevel(s string) (slog.Level, error) {
	var l slog.Level
	if err := l.UnmarshalText([]byte(s)); err != nil {
		return l, fmt.Errorf("LOG_LEVEL: %w", err)
	}
	return l, nil
}

// Int читает целое из env с умолчанием (для cmd-утилит).
func Int(key string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil {
		return v
	}
	return def
}
