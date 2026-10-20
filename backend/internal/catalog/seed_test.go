package catalog_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zbekxzz/gulder/backend/internal/catalog"
	"github.com/zbekxzz/gulder/backend/internal/testdb"
)

const dataDir = "../../../data"

func TestSeedIsIdempotentAndKeepsManualEdits(t *testing.T) {
	st, _ := testdb.New(t)
	ctx := context.Background()

	var first catalog.SeedReport
	require.NoError(t, st.InTx(ctx, func(tx pgx.Tx) error {
		var err error
		first, err = catalog.Seed(ctx, tx, dataDir)
		return err
	}))
	// §12: ≥40 букетов в ≥5 магазинах, ≥30 подарков
	assert.GreaterOrEqual(t, first.Bouquets, 40)
	assert.GreaterOrEqual(t, first.Shops, 5)
	assert.GreaterOrEqual(t, first.Gifts, 30)
	assert.Equal(t, 16, first.Taxa)

	// ручная правка флориста не затирается повторным сидом
	_, err := st.Pool.Exec(ctx, `UPDATE bouquets SET name = 'Изменено вручную' WHERE sku = 'AB-001'`)
	require.NoError(t, err)

	var second catalog.SeedReport
	require.NoError(t, st.InTx(ctx, func(tx pgx.Tx) error {
		var err error
		second, err = catalog.Seed(ctx, tx, dataDir)
		return err
	}))
	assert.Equal(t, catalog.SeedReport{}, second)

	var name string
	require.NoError(t, st.Pool.QueryRow(ctx, `SELECT name FROM bouquets WHERE sku = 'AB-001'`).Scan(&name))
	assert.Equal(t, "Изменено вручную", name)

	// FR-C03: неизвестный род → taxon_id NULL, taxon_confirmed=false
	var unconfirmed int
	require.NoError(t, st.Pool.QueryRow(ctx, `SELECT count(*) FROM flowers WHERE taxon_id IS NULL AND NOT taxon_confirmed`).Scan(&unconfirmed))
	assert.Equal(t, 2, unconfirmed)

	// скидки: есть и действующие, и истёкшие (§12)
	var active, expired int
	require.NoError(t, st.Pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE discount_until > now()),
		count(*) FILTER (WHERE discount_until <= now()) FROM price_offers WHERE discount_price IS NOT NULL`).Scan(&active, &expired))
	assert.Positive(t, active)
	assert.Positive(t, expired)
}

// M7: справочник обновляется сидом из репозитория (сверка ASPCA), и каждая строка, кроме спорной, сверена.
func TestTaxaVerifiedAgainstASPCA(t *testing.T) {
	st, _ := testdb.New(t)
	ctx := context.Background()
	require.NoError(t, st.InTx(ctx, func(tx pgx.Tx) error {
		_, err := catalog.Seed(ctx, tx, dataDir)
		return err
	}))
	// откатить сверку вручную — повторный сид восстанавливает значения из репозитория
	_, err := st.Pool.Exec(ctx, `UPDATE plant_taxa SET toxicity_verified = false, aspca_url = NULL WHERE genus = 'Rosa'`)
	require.NoError(t, err)
	var rep catalog.SeedReport
	require.NoError(t, st.InTx(ctx, func(tx pgx.Tx) error {
		var err error
		rep, err = catalog.Seed(ctx, tx, dataDir)
		return err
	}))
	assert.Equal(t, 1, rep.Taxa)

	var withoutURL int
	var unverified []string
	require.NoError(t, st.Pool.QueryRow(ctx, `SELECT count(*) FROM plant_taxa WHERE aspca_url IS NULL`).Scan(&withoutURL))
	rows, err := st.Pool.Query(ctx, `SELECT genus FROM plant_taxa WHERE NOT toxicity_verified ORDER BY genus`)
	require.NoError(t, err)
	unverified, err = pgx.CollectRows(rows, pgx.RowTo[string])
	require.NoError(t, err)
	assert.Zero(t, withoutURL, "у каждой строки есть aspca_url")
	assert.Equal(t, []string{"Gypsophila"}, unverified, "спорная строка ждёт проверки человеком (docs/aspca_verification.md)")
}
