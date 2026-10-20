package auth_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zbekxzz/gulder/backend/internal/auth"
	"github.com/zbekxzz/gulder/backend/internal/catalog"
	"github.com/zbekxzz/gulder/backend/internal/testdb"
)

// Повторный сид не падает и не дублирует пользователей (§18.3).
func TestSeedDevUsersIdempotent(t *testing.T) {
	st, _ := testdb.New(t)
	ctx := context.Background()
	run := func() int {
		var n int
		require.NoError(t, st.InTx(ctx, func(tx pgx.Tx) error {
			if _, err := catalog.Seed(ctx, tx, "../../../data"); err != nil {
				return err
			}
			var err error
			n, err = auth.SeedDevUsers(ctx, tx)
			return err
		}))
		return n
	}
	assert.Equal(t, len(auth.DevUsers), run())
	assert.Equal(t, 0, run())
	u, err := auth.Authenticate(ctx, st.Pool, "florist@gulder.local", auth.DevPassword)
	require.NoError(t, err)
	assert.NotNil(t, u.ShopID)
}
