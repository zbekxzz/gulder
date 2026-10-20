package ranking

import (
	"slices"
	"strings"
	"time"

	"github.com/zbekxzz/gulder/backend/internal/domain"
)

// NumerologyLabel — пометка на карточке (FR-N05).
const NumerologyLabel = "развлекательная функция"

// NumerologyEntry — строка активной версии таблицы «число → цветы, цвета, значение» (FR-N03).
type NumerologyEntry struct {
	VersionID int64
	Number    int
	Flowers   []string // роды, lowercase
	Colors    []string
	Meaning   string
}

// NumerologyNumber — сумма всех цифр YYYYMMDD; пока > 9 и не 11/22/33 — снова сумма цифр (FR-N02).
func NumerologyNumber(d time.Time) int {
	n := digitSum(d.Year()) + digitSum(int(d.Month())) + digitSum(d.Day())
	for n > 9 && n != 11 && n != 22 && n != 33 {
		n = digitSum(n)
	}
	return n
}

func digitSum(n int) int {
	s := 0
	for n > 0 {
		s += n % 10
		n /= 10
	}
	return s
}

// BirthDate возвращает известную дату рождения из профиля (FR-N07).
func BirthDate(p domain.Profile) (time.Time, bool) {
	s, ok := p.String(domain.AttrBirthDate)
	if !ok {
		return time.Time{}, false
	}
	d, err := time.Parse("2006-01-02", s)
	return d, err == nil
}

// numerologyFactor — доля стеблей, чей род или цвет есть в строке таблицы ∈ [0,1] (§8.3).
func numerologyFactor(b domain.Bouquet, e *NumerologyEntry) float64 {
	total := b.TotalStems()
	if e == nil || total == 0 {
		return 0
	}
	hit := 0
	for _, it := range b.Items {
		genus := ""
		if it.Flower.Taxon != nil {
			genus = strings.ToLower(it.Flower.Taxon.Genus)
		}
		if slices.Contains(e.Flowers, genus) || slices.Contains(e.Colors, strings.ToLower(it.Flower.Color)) {
			hit += it.Stems
		}
	}
	return float64(hit) / float64(total)
}
