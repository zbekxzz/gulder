package rules_test

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zbekxzz/gulder/backend/internal/catalog"
	"github.com/zbekxzz/gulder/backend/internal/domain"
)

// Справочник растений берётся из data/plant_taxa.csv (Приложение B), а не дублируется в тестах.
func loadTaxa(t testing.TB) map[string]*domain.PlantTaxon {
	t.Helper()
	f, err := os.Open("../../../data/plant_taxa.csv")
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	rows, errs, err := catalog.ParseTaxa(f)
	require.NoError(t, err)
	require.Empty(t, errs)
	out := map[string]*domain.PlantTaxon{}
	for i := range rows {
		tx := rows[i].Taxon
		out[strings.ToLower(tx.Genus)] = &tx
	}
	return out
}

type fixtures struct {
	taxa map[string]*domain.PlantTaxon
	next int64
}

func newFixtures(t testing.TB) *fixtures { return &fixtures{taxa: loadTaxa(t), next: 100} }

// flower создаёт подтверждённый цветок рода genus.
func (fx *fixtures) flower(genus, name, color string, scent domain.Level) domain.Flower {
	fx.next++
	t, ok := fx.taxa[genus]
	if !ok {
		panic("unknown genus in fixtures: " + genus)
	}
	return domain.Flower{ID: fx.next, TradeName: name, Color: color, Scent: scent, Taxon: t, TaxonConfirmed: true}
}

func (fx *fixtures) rose() domain.Flower {
	return fx.flower("rosa", "Роза", "red", domain.LevelLow)
}

func (fx *fixtures) lily() domain.Flower {
	return fx.flower("lilium", "Лилия восточная", "white", domain.LevelStrong)
}

func (fx *fixtures) gerbera() domain.Flower {
	return fx.flower("gerbera", "Гербера", "pink", domain.LevelNone)
}

func (fx *fixtures) chrysanthemum() domain.Flower {
	return fx.flower("chrysanthemum", "Хризантема", "white", domain.LevelLow)
}

func (fx *fixtures) orchid() domain.Flower {
	return fx.flower("phalaenopsis", "Орхидея", "white", domain.LevelNone)
}

func (fx *fixtures) eucalyptus() domain.Flower {
	return fx.flower("eucalyptus", "Эвкалипт", "green", domain.LevelMedium)
}

func bouquet(id int64, name string, price int64, items ...domain.BouquetItem) domain.Bouquet {
	return domain.Bouquet{ID: id, Name: name, Items: items, Price: domain.Tenge(price)}
}

func item(f domain.Flower, stems int) domain.BouquetItem {
	return domain.BouquetItem{Flower: f, Stems: stems}
}

func codes(c ...string) *[]string { return &c }

func boolPtr(b bool) *bool { return &b }

func gift(id int64, name, category string, price int64, allergens *[]string, scent domain.Level, petHazard *bool) domain.GiftItem {
	return domain.GiftItem{
		ID: id, Name: name, Category: category, Price: domain.Tenge(price),
		AllergenCodes: allergens, Scent: scent, PetHazard: petHazard,
	}
}

func a(v any, conf domain.Confidence) domain.Attribute {
	return domain.Attribute{Value: v, Source: domain.SourceGiver, Confidence: conf}
}

// noRestrictions — «всё известно, ограничений нет».
func noRestrictions() domain.Profile {
	return domain.Profile{
		domain.AttrAllergies:        a([]any{}, domain.Know),
		domain.AttrPets:             a([]any{}, domain.Know),
		domain.AttrAsthma:           a(false, domain.Know),
		domain.AttrScentSensitivity: a("none", domain.Know),
	}
}

func with(p domain.Profile, key string, attr domain.Attribute) domain.Profile {
	out := domain.Profile{}
	for k, v := range p {
		out[k] = v
	}
	out[key] = attr
	return out
}

func without(p domain.Profile, key string) domain.Profile {
	out := domain.Profile{}
	for k, v := range p {
		if k != key {
			out[k] = v
		}
	}
	return out
}
