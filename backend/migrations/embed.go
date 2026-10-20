// Package migrations встраивает SQL-миграции goose в бинарник (§18.3).
package migrations

import "embed"

// FS содержит все *.sql миграции.
//
//go:embed *.sql
var FS embed.FS
