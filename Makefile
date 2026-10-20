# Gulder — основные команды (§3). Требуется Docker (для make dev) и/или Go + Node локально.
SHELL := /bin/bash
.DEFAULT_GOAL := help

COMPOSE        ?= docker compose
DB_PORT        ?= 5433
DATABASE_URL   ?= postgres://gulder:gulder@localhost:$(DB_PORT)/gulder?sslmode=disable
EVAL_SEED      ?= 42
GOLANGCI       := go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0
GOVULNCHECK    := go run golang.org/x/vuln/cmd/govulncheck@v1.9.0

export DATABASE_URL

.PHONY: help
help: ## Показать список команд
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

# ─────────────── Запуск ───────────────
.PHONY: dev down logs dev-api dev-web
dev: ## Поднять стек (postgres, migrate, backend, frontend) в Docker
	$(COMPOSE) up --build -d
	@echo "Frontend: http://localhost:5173  API: http://localhost:8080"

down: ## Остановить стек
	$(COMPOSE) down

logs: ## Логи стека
	$(COMPOSE) logs -f --tail=100

dev-api: ## Запустить API локально (без Docker), нужен DATABASE_URL
	cd backend && go run ./cmd/migrate up && go run ./cmd/api

dev-web: ## Запустить фронтенд локально (Vite, прокси /api → :8080)
	cd frontend && npm run dev

# ─────────────── Данные ───────────────
.PHONY: migrate migrate-down migrate-reset seed eval
migrate: ## Применить миграции
	cd backend && go run ./cmd/migrate up

migrate-down: ## Откатить последнюю миграцию
	cd backend && go run ./cmd/migrate down

migrate-reset: ## Откатить все миграции
	cd backend && go run ./cmd/migrate reset

seed: ## Загрузить data/*.csv (идемпотентно); в Docker, если стек запущен
	@if $(COMPOSE) ps --status running -q backend 2>/dev/null | grep -q .; then \
		$(COMPOSE) exec -T backend /app/seed -data /app/data; \
	else \
		cd backend && go run ./cmd/seed -data ../data; \
	fi

eval: ## Офлайн-оценка ranker (§11), воспроизводима по EVAL_SEED
	cd backend && go run ./cmd/eval -seed $(EVAL_SEED) -report ../docs/eval_report.md

# ─────────────── Эксплуатация ───────────────
.PHONY: backup restore prod-config
backup: ## Бэкап БД (scripts/backup.sh), см. docs/runbook.md
	scripts/backup.sh

restore: ## Восстановить DUMP=<файл> в пустую TARGET_URL
	scripts/restore.sh $(DUMP)

prod-config: ## Проверить docker-compose.prod.yml
	$(COMPOSE) -f docker-compose.prod.yml config --quiet

# ─────────────── Проверки ───────────────
.PHONY: test test-go test-web test-e2e lint lint-go lint-web vuln build gen-api fmt
test: test-go test-web test-e2e ## Все тесты: Go, vitest, Playwright

test-go: ## Go-тесты (-race -cover); интеграционные — TEST_DATABASE_URL или Docker
	cd backend && go test ./... -race -cover

test-web: ## vitest
	cd frontend && npm test

test-e2e: ## Playwright (поднимает API и Vite сам, нужен E2E_DATABASE_URL или Docker-стек)
	cd frontend && npm run e2e

lint: lint-go lint-web ## Все линтеры

lint-go:
	cd backend && $(GOLANGCI) run ./...

lint-web:
	cd frontend && npm run lint

vuln: ## govulncheck
	cd backend && $(GOVULNCHECK) ./...

build: ## Сборка бэкенда и фронтенда
	cd backend && go build ./...
	cd frontend && npm run build

gen-api: ## Сгенерировать TS-типы из backend/api/openapi.yaml
	cd frontend && npm run gen:api

fmt: ## Форматирование
	cd backend && $(GOLANGCI) fmt ./...
	cd frontend && npm run format
