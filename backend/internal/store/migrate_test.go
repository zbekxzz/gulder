package store_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zbekxzz/gulder/backend/internal/store"
	"github.com/zbekxzz/gulder/backend/internal/testdb"
)

// Каждая миграция обратима: up → reset → up на чистой базе (§18.3).
func TestMigrationsUpDownUp(t *testing.T) {
	st, url := testdb.New(t)
	ctx := context.Background()

	var tables int
	require.NoError(t, st.Pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables
		WHERE table_schema IN ('public','research') AND table_type='BASE TABLE' AND table_name <> 'goose_db_version'`).Scan(&tables))
	assert.GreaterOrEqual(t, tables, 25)

	require.NoError(t, store.Migrate(ctx, url, "reset"))
	var left int
	require.NoError(t, st.Pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables
		WHERE table_schema IN ('public','research') AND table_name <> 'goose_db_version'`).Scan(&left))
	assert.Equal(t, 0, left, "down must remove every object")
	require.NoError(t, store.Migrate(ctx, url, "up"))

	ok, err := st.MigrationsCurrent(ctx)
	require.NoError(t, err)
	assert.True(t, ok)
}

// CHECK на «know без значения» из Приложения A (FR-P03, AC-2 на уровне БД).
func TestAC2_DBRejectsKnowWithoutValue(t *testing.T) {
	st, _ := testdb.New(t)
	ctx := context.Background()
	var uid, rid int64
	require.NoError(t, st.Pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('a@b.c') RETURNING id`).Scan(&uid))
	require.NoError(t, st.Pool.QueryRow(ctx, `INSERT INTO recipients (giver_id, nickname) VALUES ($1,'x') RETURNING id`, uid).Scan(&rid))
	_, err := st.Pool.Exec(ctx, `INSERT INTO recipient_attributes (recipient_id, key, value, source, confidence)
		VALUES ($1,'allergies',NULL,'giver','know')`, rid)
	require.Error(t, err)
	_, err = st.Pool.Exec(ctx, `INSERT INTO recipient_attributes (recipient_id, key, value, source, confidence)
		VALUES ($1,'allergies',NULL,'giver','unknown')`, rid)
	require.NoError(t, err)
}
