package offers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHaversine(t *testing.T) {
	// Абая 52 → Толе би 101 в Алматы ≈ 2.9 км
	d := HaversineMeters(Point{43.2401, 76.9264}, Point{43.2531, 76.9030})
	assert.InDelta(t, 2400, d, 400)
	assert.InDelta(t, 0, HaversineMeters(Point{43, 76}, Point{43, 76}), 1e-9)
	// симметрия
	assert.InDelta(t, d, HaversineMeters(Point{43.2531, 76.9030}, Point{43.2401, 76.9264}), 1e-6)
}

func TestParseFilters(t *testing.T) {
	f, err := ParseFilters(" nearby,discount,nearby ")
	require.NoError(t, err)
	assert.Equal(t, []string{FilterNearby, FilterDiscount}, f)
	_, err = ParseFilters("cheap")
	assert.Error(t, err)
	f, err = ParseFilters("")
	require.NoError(t, err)
	assert.Empty(t, f)
}

func TestCompareDistance(t *testing.T) {
	one, two := 1.0, 2.0
	assert.Equal(t, -1, compareDistance(&one, &two))
	assert.Equal(t, 1, compareDistance(nil, &one))
	assert.Equal(t, -1, compareDistance(&one, nil))
	assert.Equal(t, 0, compareDistance(nil, nil))
}
