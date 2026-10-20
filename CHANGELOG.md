# Changelog

Формат — [Keep a Changelog](https://keepachangelog.com/ru/1.1.0/), версии — [SemVer](https://semver.org/lang/ru/).

## [Unreleased]

### Added
- M0: монорепозиторий, миграция схемы из Приложения A (goose), сид каталога из `data/*.csv`,
  docker-compose, Makefile, CI (lint, тесты с `-race`, govulncheck, миграции up/down/up, Playwright).
- M1: JWT-аутентификация и роли, получатели с профилем value/source/confidence, поводы,
  импорт каталога CSV с построчным отчётом, наличие и цены, audit log.
