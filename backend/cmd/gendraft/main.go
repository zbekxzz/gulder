// Command gendraft генерирует ЧЕРНОВОЙ экспертный набор data/expert_judgements.csv (dataset_version=draft-1, §12).
//
// Это не экспертная разметка: оценки 0–3 выставляет простая эвристика (любимые цветы и цвета, бюджет,
// нелюбимое), а небезопасные пары помечает Rules Engine по профилю случая. Черновик нужен, чтобы отладить
// пайплайн оценки; на вехе M7 флорист проверяет разметку, и она сохраняется как версия v1.
// Генерация детерминирована по -seed.
package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/zbekxzz/gulder/backend/internal/catalog"
	"github.com/zbekxzz/gulder/backend/internal/domain"
	"github.com/zbekxzz/gulder/backend/internal/rules"
)

func main() {
	dataDir := flag.String("data", "../data", "каталог с CSV")
	seed := flag.Uint64("seed", 2026, "seed генератора")
	recipients := flag.Int("recipients", 36, "число получателей")
	flag.Parse()
	if err := run(*dataDir, *seed, *recipients); err != nil {
		fmt.Fprintln(os.Stderr, "gendraft:", err)
		os.Exit(1)
	}
}

type bouquet struct {
	shop, sku string
	b         domain.Bouquet
}

func parse[T any](dir, name string, fn func(io.Reader) ([]T, []catalog.RowError, error)) ([]T, error) {
	f, err := os.Open(filepath.Join(dir, name)) //nolint:gosec // путь задаёт оператор
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	rows, errs, err := fn(f)
	if err != nil {
		return nil, err
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("%s: %v", name, errs)
	}
	return rows, nil
}

func loadCatalog(dir string) ([]bouquet, error) {
	taxa, err := parse(dir, "plant_taxa.csv", catalog.ParseTaxa)
	if err != nil {
		return nil, err
	}
	byGenus := map[string]*domain.PlantTaxon{}
	bySci := map[string]*domain.PlantTaxon{}
	for i := range taxa {
		t := taxa[i].Taxon
		byGenus[strings.ToLower(t.Genus)] = &t
		bySci[strings.ToLower(t.ScientificName)] = &t
	}
	flowers, err := parse(dir, "flowers.csv", catalog.ParseFlowers)
	if err != nil {
		return nil, err
	}
	fl := map[string]domain.Flower{}
	for _, f := range flowers {
		t := bySci[strings.ToLower(f.ScientificName)]
		if t == nil {
			t = byGenus[strings.ToLower(f.Genus)]
		}
		fl[f.Code] = domain.Flower{Code: f.Code, TradeName: f.TradeName, Color: f.Color, Scent: f.Scent, Taxon: t, TaxonConfirmed: f.TaxonConfirmed && t != nil}
	}
	rows, err := parse(dir, "bouquets.csv", catalog.ParseBouquets)
	if err != nil {
		return nil, err
	}
	items, err := parse(dir, "bouquet_items.csv", catalog.ParseBouquetItems)
	if err != nil {
		return nil, err
	}
	var out []bouquet
	for i, r := range rows {
		b := domain.Bouquet{ID: int64(i + 1), Name: r.Name, Price: r.Offer.Price, Style: r.Style, CompositionFormat: r.CompositionFormat, InStock: r.InStock}
		for _, it := range items {
			if it.ShopCode == r.ShopCode && it.SKU == r.SKU {
				b.Items = append(b.Items, domain.BouquetItem{Flower: fl[it.Item.FlowerCode], Stems: it.Item.Stems})
			}
		}
		out = append(out, bouquet{shop: r.ShopCode, sku: r.SKU, b: b})
	}
	return out, nil
}

// snapshotAttr — поле профиля в recipient_snapshot (как в БД: value/source/confidence).
type snapshotAttr struct {
	Value      any    `json:"value"`
	Source     string `json:"source"`
	Confidence string `json:"confidence"`
}

var (
	allergenPool = []string{"genus:lilium", "family:asteraceae", "pollen", "food:nuts", "food:gluten", "food:milk", "fragrance", "genus:rosa"}
	petPool      = []string{"cat", "dog", "rabbit"}
	generaPool   = []string{"rosa", "lilium", "tulipa", "gerbera", "paeonia", "phalaenopsis", "hydrangea", "alstroemeria", "chrysanthemum", "dianthus", "helianthus", "freesia"}
	colorPool    = []string{"white", "pink", "red", "yellow", "peach", "orange", "blue", "purple"}
	occasions    = []string{"birthday", "anniversary", "march8", "gratitude", "apology", "congrats", "sympathy", "just_because", "valentine", "wedding"}
	emotionPool  = []string{"joy", "gratitude", "romance", "support", "respect", "apology"}
	stylesPool   = []string{"classic", "minimal", "rustic", "bright"}
)

func pickN(rng *rand.Rand, pool []string, n int) []any {
	idx := rng.Perm(len(pool))[:n]
	out := make([]any, 0, n)
	for _, i := range idx {
		out = append(out, pool[i])
	}
	return out
}

// randomProfile — разнообразие из §12: аллергии, питомцы, неизвестные поля.
func randomProfile(rng *rand.Rand) map[string]snapshotAttr {
	p := map[string]snapshotAttr{}
	conf := func() string {
		switch r := rng.IntN(10); {
		case r < 6:
			return "know"
		case r < 8:
			return "assume"
		default:
			return "unknown"
		}
	}
	set := func(key string, v any) {
		c := conf()
		if c == "unknown" {
			return // отсутствие = unknown
		}
		p[key] = snapshotAttr{Value: v, Source: "giver", Confidence: c}
	}
	set("allergies", pickN(rng, allergenPool, rng.IntN(3)))
	set("pets", pickN(rng, petPool, rng.IntN(2)))
	set("asthma", rng.IntN(6) == 0)
	set("scent_sensitivity", []string{"none", "low", "medium", "strong"}[rng.IntN(4)])
	if rng.IntN(3) > 0 {
		set("liked_flowers", pickN(rng, generaPool, 1+rng.IntN(2)))
	}
	if rng.IntN(2) == 0 {
		set("disliked_flowers", pickN(rng, generaPool, 1))
	}
	if rng.IntN(3) > 0 {
		set("liked_colors", pickN(rng, colorPool, 1+rng.IntN(2)))
	}
	if rng.IntN(3) == 0 {
		set("style", stylesPool[rng.IntN(len(stylesPool))])
	}
	return p
}

func toProfile(snap map[string]snapshotAttr) domain.Profile {
	p := domain.Profile{}
	for k, a := range snap {
		p[k] = domain.Attribute{Value: a.Value, Source: domain.Source(a.Source), Confidence: domain.Confidence(a.Confidence)}
	}
	return p
}

func shareOf(b domain.Bouquet, set []string, key func(domain.BouquetItem) string) float64 {
	total, hit := 0, 0
	for _, it := range b.Items {
		total += it.Stems
		if slices.Contains(set, key(it)) {
			hit += it.Stems
		}
	}
	if total == 0 {
		return 0
	}
	return float64(hit) / float64(total)
}

func genus(it domain.BouquetItem) string {
	if it.Flower.Taxon == nil {
		return ""
	}
	return strings.ToLower(it.Flower.Taxon.Genus)
}

func color(it domain.BouquetItem) string { return it.Flower.Color }

// grade — черновая эвристика оценки 0–3 с причиной.
func grade(b domain.Bouquet, p domain.Profile, budgetMax domain.Money, occasion string) (int, bool, string) {
	if res := rules.CheckBouquet(b, p); res.Blocked() {
		return 0, true, "Небезопасно по профилю: " + strings.Join(res.Codes(), ", ")
	}
	if b.Price > budgetMax {
		return 0, false, "Дороже бюджета"
	}
	g := 1
	var why []string
	if liked, ok := p.StringList(domain.AttrLikedFlowers); ok && shareOf(b, liked, genus) >= 0.5 {
		g++
		why = append(why, "любимые цветы")
	}
	if liked, ok := p.StringList(domain.AttrLikedColors); ok && shareOf(b, liked, color) >= 0.5 {
		g++
		why = append(why, "любимые цвета")
	}
	if disliked, ok := p.StringList(domain.AttrDislikedFlowers); ok && shareOf(b, disliked, genus) > 0 {
		g--
		why = append(why, "есть нелюбимые цветы")
	}
	if occasion == "sympathy" && b.Style == "bright" {
		g--
		why = append(why, "слишком ярко для повода")
	}
	g = max(0, min(3, g))
	if len(why) == 0 {
		why = append(why, "нейтрально")
	}
	return g, false, "Черновик: " + strings.Join(why, ", ")
}

func run(dataDir string, seed uint64, nRecipients int) error {
	cat, err := loadCatalog(dataDir)
	if err != nil {
		return err
	}
	rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))          //nolint:gosec // детерминированность по seed — требование
	f, err := os.Create(filepath.Join(dataDir, "expert_judgements.csv")) //nolint:gosec // путь задаёт оператор
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	w := csv.NewWriter(f)
	_ = w.Write([]string{"dataset_version", "case_id", "recipient_snapshot", "occasion_snapshot", "shop_code", "sku", "grade", "is_unsafe", "reason"})
	rows := 0
	for r := 1; r <= nRecipients; r++ {
		profileSnap := randomProfile(rng)
		profile := toProfile(profileSnap)
		cases := 1
		if r%3 == 0 {
			cases = 2 // у части получателей несколько поводов: split по получателю, а не по случаю
		}
		for c := 1; c <= cases; c++ {
			budget := domain.Tenge(int64(10000 + 5000*rng.IntN(8)))
			occ := map[string]any{
				"type": occasions[rng.IntN(len(occasions))], "emotion_goal": emotionPool[rng.IntN(len(emotionPool))],
				"budget_min": 0, "budget_max": budget, "date": "2026-11-15",
			}
			occJSON, _ := json.Marshal(occ)
			profJSON, _ := json.Marshal(profileSnap)
			// 5 букетов на случай: по любимым цветам и цветам (если есть), один небезопасный, остальные — допустимые в бюджете
			chosen := map[int]bool{}
			fits := func(i int) bool {
				return !rules.CheckBouquet(cat[i].b, profile).Blocked() && cat[i].b.Price <= budget
			}
			if liked, ok := profile.StringList(domain.AttrLikedFlowers); ok {
				for _, i := range rng.Perm(len(cat)) {
					if fits(i) && shareOf(cat[i].b, liked, genus) >= 0.5 {
						chosen[i] = true
						break
					}
				}
			}
			if liked, ok := profile.StringList(domain.AttrLikedColors); ok {
				for _, i := range rng.Perm(len(cat)) {
					if fits(i) && !chosen[i] && shareOf(cat[i].b, liked, color) >= 0.5 {
						chosen[i] = true
						break
					}
				}
			}
			for _, i := range rng.Perm(len(cat)) { // одна небезопасная пара — для unsafe rate
				if rules.CheckBouquet(cat[i].b, profile).Blocked() {
					chosen[i] = true
					break
				}
			}
			for _, i := range rng.Perm(len(cat)) {
				if len(chosen) >= 5 {
					break
				}
				if fits(i) {
					chosen[i] = true
				}
			}
			idx := make([]int, 0, len(chosen))
			for i := range chosen {
				idx = append(idx, i)
			}
			slices.Sort(idx)
			caseID := fmt.Sprintf("r%02d-c%d", r, c)
			for _, i := range idx {
				g, unsafe, reason := grade(cat[i].b, profile, budget, occ["type"].(string))
				_ = w.Write([]string{
					"draft-1", caseID, string(profJSON), string(occJSON), cat[i].shop, cat[i].sku,
					fmt.Sprint(g), fmt.Sprint(unsafe), reason,
				})
				rows++
			}
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return err
	}
	fmt.Printf("gendraft: %d rows, %d recipients\n", rows, nRecipients)
	return nil
}
