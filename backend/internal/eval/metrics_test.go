package eval

import (
	"fmt"
	"math"
	"math/rand/v2"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNDCG(t *testing.T) {
	assert.InDelta(t, 1, NDCGAt(5, []int{3, 2, 1}, []int{1, 2, 3}), 1e-12, "идеальный порядок")
	worst := NDCGAt(5, []int{0, 1, 3}, []int{3, 1, 0})
	assert.Less(t, worst, 1.0)
	// ручной расчёт: DCG = 0 + 1/log2(3) + 7/2, IDCG = 7 + 1/log2(3)
	want := (1/math.Log2(3) + 7.0/2) / (7 + 1/math.Log2(3))
	assert.InDelta(t, want, worst, 1e-12)
	assert.InDelta(t, 1, NDCGAt(5, []int{0, 0}, []int{0, 0}), 1e-12, "нет релевантных")
	// отсечка k: релевантный на 6-м месте не учитывается
	assert.InDelta(t, 0, NDCGAt(5, []int{0, 0, 0, 0, 0, 3}, []int{3, 0, 0, 0, 0, 0}), 1e-12)
}

func TestBootstrapDeterministic(t *testing.T) {
	groups := []float64{0.2, 0.4, 0.6, 0.8, 1.0}
	a := Bootstrap(groups, 1000, 42)
	b := Bootstrap(groups, 1000, 42)
	assert.Equal(t, a, b, "фиксированный seed — тот же CI")
	assert.InDelta(t, 0.6, a.Mean, 1e-12)
	assert.Less(t, a.CI95[0], a.Mean)
	assert.Greater(t, a.CI95[1], a.Mean)
	assert.Equal(t, CI{}, Bootstrap(nil, 10, 1))
}

func TestECE(t *testing.T) {
	assert.InDelta(t, 0, ECE([]float64{0.95, 0.95, 0.05, 0.05}, []bool{true, true, false, false}, 10), 0.051)
	assert.InDelta(t, 0.9, ECE([]float64{0.95, 0.95}, []bool{false, false}, 10), 0.051)
	assert.Equal(t, 0.0, ECE(nil, nil, 10))
}

func TestFitPlattRecoversCalibration(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	var x []float64
	var y []bool
	// истинная вероятность 0.2 + 0.6x — модель «пере-уверена» на краях
	for i := 0; i < 4000; i++ {
		xi := rng.Float64()
		x = append(x, xi)
		y = append(y, rng.Float64() < 0.2+0.6*xi)
	}
	p := FitPlatt(x, y)
	cal := make([]float64, len(x))
	for i, xi := range x {
		cal[i] = p.Apply(xi)
	}
	assert.Less(t, ECE(cal, y, 10), ECE(x, y, 10))
	assert.Greater(t, p.A, 0.0)
}

// §11: один получатель не может быть и в train, и в test.
func TestGroupSplitByRecipient(t *testing.T) {
	var cases []Case
	for r := 1; r <= 60; r++ {
		for c := 1; c <= 1+r%3; c++ {
			id := fmt.Sprintf("r%02d-c%d", r, c)
			cases = append(cases, Case{ID: id, RecipientKey: RecipientKey(id)})
		}
	}
	train, test := GroupSplit(cases, 42, 0.3)
	require.NotEmpty(t, train)
	require.NotEmpty(t, test)
	assert.Len(t, append(train, test...), len(cases))
	inTrain := map[string]bool{}
	for _, c := range train {
		inTrain[c.RecipientKey] = true
	}
	for _, c := range test {
		assert.False(t, inTrain[c.RecipientKey], "получатель %s и в train, и в test", c.RecipientKey)
	}
	frac := float64(len(test)) / float64(len(cases))
	assert.InDelta(t, 0.3, frac, 0.15)
	train2, test2 := GroupSplit(cases, 42, 0.3)
	assert.Equal(t, train, train2)
	assert.Equal(t, test, test2)
	assert.Equal(t, "r07", RecipientKey("r07-c2"))
}
