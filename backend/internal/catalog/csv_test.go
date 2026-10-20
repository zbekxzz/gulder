package catalog

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseGiftsAllergenSemantics(t *testing.T) {
	csv := "sku,name,category,allergen_codes,price\n" +
		"A,Конфеты,sweets,,1000\n" + // пусто = неизвестно
		"B,Открытка,card,-,500\n" + // "-" = проверено, аллергенов нет
		"C,Пралине,sweets,food:nuts:hazelnut;FOOD:milk,2000\n"
	rows, errs, err := ParseGifts(strings.NewReader(csv))
	require.NoError(t, err)
	require.Empty(t, errs)
	assert.Nil(t, rows[0].AllergenCodes)
	require.NotNil(t, rows[1].AllergenCodes)
	assert.Empty(t, *rows[1].AllergenCodes)
	assert.Equal(t, []string{"food:nuts:hazelnut", "food:milk"}, *rows[2].AllergenCodes)
}

func TestParseBouquetsReportsErrorsPerLine(t *testing.T) {
	csv := "sku,name,price,discount_price,discount_until,composition\n" +
		"OK-1,Хороший,10000,,,rose_red:5\n" +
		"BAD-1,,abc,,,rose_red:5\n" +
		"BAD-2,Скидка без срока,10000,9000,,rose_red:x\n"
	_, errs, err := ParseBouquets(strings.NewReader(csv))
	require.NoError(t, err)
	byLine := map[int][]string{}
	for _, e := range errs {
		byLine[e.Line] = append(byLine[e.Line], e.Field)
	}
	assert.NotContains(t, byLine, 2)
	assert.ElementsMatch(t, []string{"name", "price"}, byLine[3])
	assert.ElementsMatch(t, []string{"discount_until", "composition"}, byLine[4])
}

func TestParseMissingColumns(t *testing.T) {
	_, _, err := ParseGifts(strings.NewReader("sku,name\nA,B\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "category")
}

func TestOpeningHours(t *testing.T) {
	h, err := ParseOpeningHours([]byte(`{"mon":[["09:00","13:00"],["14:00","21:00"]],"sun":[]}`))
	require.NoError(t, err)
	loc, _ := time.LoadLocation("Asia/Almaty")
	mon := time.Date(2026, 10, 19, 10, 0, 0, 0, loc) // понедельник
	assert.True(t, h.OpenAt(mon, loc))
	assert.False(t, h.OpenAt(mon.Add(3*time.Hour+30*time.Minute), loc), "перерыв 13–14")
	assert.False(t, h.OpenAt(mon.Add(-2*time.Hour), loc))
	assert.False(t, h.OpenAt(mon.AddDate(0, 0, -1), loc), "воскресенье — выходной")
	// время в UTC пересчитывается в часовой пояс магазина
	assert.True(t, h.OpenAt(mon.UTC(), loc))

	_, err = ParseOpeningHours([]byte(`{"mon":[["21:00","09:00"]]}`))
	assert.Error(t, err)
	_, err = ParseOpeningHours([]byte(`{"xyz":[]}`))
	assert.Error(t, err)
}
