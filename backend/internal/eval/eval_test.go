package eval_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zbekxzz/gulder/backend/internal/catalog"
	"github.com/zbekxzz/gulder/backend/internal/eval"
	"github.com/zbekxzz/gulder/backend/internal/ranking"
	"github.com/zbekxzz/gulder/backend/internal/testdb"
)

// AC-9 и §11: оценка на черновом наборе — unsafe = 0, бюджет не нарушается, coverage ≥ 30%,
// калибровка снижает ECE; запуск воспроизводим по seed и пишется в research.experiment_runs.
func TestAC9_EvaluationPipeline(t *testing.T) {
	st, _ := testdb.New(t)
	ctx := context.Background()
	require.NoError(t, st.InTx(ctx, func(tx pgx.Tx) error {
		_, err := catalog.Seed(ctx, tx, "../../../data")
		return err
	}))
	version, err := eval.LatestDataset(ctx, st.Pool)
	require.NoError(t, err)
	assert.Equal(t, "draft-1", version)
	cases, err := eval.LoadCases(ctx, st.Pool, version)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(cases), 30)
	rows, recipients := 0, map[string]bool{}
	for _, c := range cases {
		rows += len(c.Judgements)
		recipients[c.RecipientKey] = true
	}
	assert.GreaterOrEqual(t, rows, 100, "§12: ≥ 100 случаев «получатель + повод + букет»")
	assert.GreaterOrEqual(t, len(recipients), 30, "§12: ≥ 30 разных получателей")

	opt := eval.DefaultOptions(42)
	opt.BootstrapIters = 200
	snap, err := catalog.LoadSnapshot(ctx, st.Pool, opt.Now)
	require.NoError(t, err)
	cfg := ranking.DefaultConfig()
	a := eval.Evaluate(ctx, cases, version, snap, cfg, opt)
	b := eval.Evaluate(ctx, cases, version, snap, cfg, opt)
	assert.Equal(t, a, b, "воспроизводимо по seed")

	m := a.Baseline
	assert.Zero(t, m.UnsafeRate)
	assert.Zero(t, m.UnsafeCatalogRate)
	assert.Zero(t, m.BudgetViolations)
	assert.GreaterOrEqual(t, m.Coverage, 0.30)
	assert.Less(t, a.Calibrated.ECE, m.ECE)
	assert.Positive(t, m.TestCases)
	assert.Equal(t, 5, m.Robustness.Repeats)

	baseID, calID, err := eval.SaveRuns(ctx, st.Pool, cfg, a)
	require.NoError(t, err)
	var n int
	var numerology bool
	require.NoError(t, st.Pool.QueryRow(ctx, `SELECT count(*), bool_or(numerology_in_features) FROM research.experiment_runs
		WHERE id IN ($1,$2) AND split_id = $3 AND seed = 42 AND dataset_version = 'draft-1'`, baseID, calID, a.SplitID).Scan(&n, &numerology))
	assert.Equal(t, 2, n, "baseline и модель на одном split (RM-05)")
	assert.False(t, numerology, "FR-N06")
}
