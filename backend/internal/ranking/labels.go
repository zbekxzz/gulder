package ranking

import "strings"

// Русские подписи для объяснений (FR-R03). Неизвестное значение показывается как есть.
var (
	genusLabels = map[string]string{
		"rosa": "розы", "lilium": "лилии", "tulipa": "тюльпаны", "chrysanthemum": "хризантемы",
		"helianthus": "подсолнухи", "gerbera": "герберы", "dianthus": "гвоздики", "phalaenopsis": "орхидеи",
		"hydrangea": "гортензии", "paeonia": "пионы", "narcissus": "нарциссы", "alstroemeria": "альстромерии",
		"gypsophila": "гипсофила", "freesia": "фрезии", "eucalyptus": "эвкалипт", "hyacinthus": "гиацинты",
	}
	colorLabels = map[string]string{
		"red": "красный", "white": "белый", "pink": "розовый", "yellow": "жёлтый", "peach": "персиковый",
		"purple": "сиреневый", "blue": "голубой", "orange": "оранжевый", "green": "зелёный", "cream": "кремовый",
	}
	occasionLabels = map[string]string{
		"birthday": "день рождения", "anniversary": "годовщина", "march8": "8 Марта", "valentine": "День влюблённых",
		"wedding": "свадьба", "apology": "извинение", "gratitude": "благодарность", "congrats": "поздравление",
		"sympathy": "соболезнование", "just_because": "без повода",
	}
	styleLabels = map[string]string{"classic": "классика", "minimal": "минимализм", "rustic": "рустик", "bright": "ярко"}
)

func label(m map[string]string, key string) string {
	if v, ok := m[strings.ToLower(key)]; ok {
		return v
	}
	return key
}

func labels(m map[string]string, keys []string) string {
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, label(m, k))
	}
	return strings.Join(out, ", ")
}

// GenusLabel — русское название рода во множественном числе.
func GenusLabel(genus string) string { return label(genusLabels, genus) }

// OccasionLabel — русское название повода.
func OccasionLabel(t string) string { return label(occasionLabels, t) }
