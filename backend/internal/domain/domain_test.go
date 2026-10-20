package domain

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseMoney(t *testing.T) {
	cases := map[string]Money{"12000": 1200000, "12000.5": 1200050, "12000.05": 1200005, "0.99": 99, "-3.10": -310}
	for in, want := range cases {
		got, err := ParseMoney(in)
		require.NoError(t, err, in)
		assert.Equal(t, want, got, in)
	}
	for _, bad := range []string{"", "1.234", "1,5", "abc", ".5", "1."} {
		_, err := ParseMoney(bad)
		assert.Error(t, err, bad)
	}
}

func TestMoneyJSONRoundTrip(t *testing.T) {
	b, err := json.Marshal(struct{ P Money }{Tenge(15000) + 50})
	require.NoError(t, err)
	assert.JSONEq(t, `{"P":15000.5}`, string(b))

	var v struct{ P Money }
	require.NoError(t, json.Unmarshal([]byte(`{"P":12900}`), &v))
	assert.Equal(t, Tenge(12900), v.P)
	require.NoError(t, json.Unmarshal([]byte(`{"P":"99.90"}`), &v))
	assert.Equal(t, Money(9990), v.P)
}

func TestLevelRank(t *testing.T) {
	assert.True(t, LevelStrong.AtLeast(LevelMedium))
	assert.False(t, LevelLow.AtLeast(LevelMedium))
	assert.Equal(t, LevelMedium, MaxLevel(LevelLow, LevelMedium))
	// неизвестное значение — консервативно strong
	assert.Equal(t, 3, Level("garbage").Rank())
}

func TestProfileKnown(t *testing.T) {
	p := Profile{
		AttrAllergies: {Value: nil, Confidence: Unknown},
		AttrPets:      {Value: []any{"Cat"}, Confidence: Assume},
		AttrAsthma:    {Value: "yes", Confidence: Know}, // неверный тип → неизвестно
	}
	_, ok := p.StringList(AttrAllergies)
	assert.False(t, ok)
	pets, ok := p.StringList(AttrPets)
	assert.True(t, ok)
	assert.Equal(t, []string{"cat"}, pets)
	_, ok = p.Bool(AttrAsthma)
	assert.False(t, ok)
	_, ok = p.Get("liked_colors")
	assert.False(t, ok)
}

func TestDiscountActive(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	dp := Tenge(900)
	from, until := now.Add(-time.Hour), now.Add(time.Hour)
	o := PriceOffer{Price: Tenge(1000), DiscountPrice: &dp, DiscountFrom: &from, DiscountUntil: &until}
	assert.True(t, o.DiscountActive(now))
	assert.Equal(t, Tenge(900), o.Effective(now))
	assert.False(t, o.DiscountActive(until), "until не включается")
	assert.Equal(t, Tenge(1000), o.Effective(until))
	assert.False(t, o.DiscountActive(from.Add(-time.Second)))
}
