package rules_test

import (
	"math/rand/v2"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zbekxzz/gulder/backend/internal/domain"
	"github.com/zbekxzz/gulder/backend/internal/rules"
)

// randomProfile генерирует произвольный профиль: каждое поле — известно/предполагается/неизвестно
// с произвольным значением. Источник тоже случайный.
func randomProfile(rng *rand.Rand) domain.Profile {
	confs := []domain.Confidence{domain.Know, domain.Assume, domain.Unknown}
	sources := []domain.Source{domain.SourceGiver, domain.SourceRecipient, domain.SourceFeedback}
	pick := func(vals ...any) any { return vals[rng.IntN(len(vals))] }
	p := domain.Profile{}
	set := func(key string, v any) {
		if rng.IntN(5) == 0 {
			return // отсутствует
		}
		c := confs[rng.IntN(len(confs))]
		if c == domain.Unknown {
			v = nil
		}
		p[key] = domain.Attribute{Value: v, Confidence: c, Source: sources[rng.IntN(len(sources))]}
	}
	set(domain.AttrAllergies, pick([]any{}, []any{"food:nuts"}, []any{"genus:rosa"}, []any{"pollen", "latex"}))
	set(domain.AttrAsthma, pick(true, false))
	set(domain.AttrScentSensitivity, pick("none", "low", "medium", "strong"))
	set(domain.AttrLikedFlowers, pick([]any{"rosa"}, []any{}))
	set(domain.AttrDislikedFlowers, pick([]any{"tulipa"}, []any{}))
	set(domain.AttrDislikedColors, pick([]any{"red"}, []any{}))
	set(domain.AttrStyle, pick("classic", "minimal"))
	return p
}

// §7.7 №18: для каждого вида cat/dog/horse и каждого таксона справочника — если токсичность ≠ none,
// однокомпонентный букет всегда BLOCK, каким бы ни был остальной профиль.
func TestPropertyToxicPlantAlwaysBlockedForOwner(t *testing.T) {
	taxa := loadTaxa(t)
	rng := rand.New(rand.NewPCG(42, 7))
	checked := 0
	for _, species := range domain.PetSpecies {
		for genus, taxon := range taxa {
			tox := taxon.PetToxicity[species]
			if tox == domain.ToxNone {
				continue
			}
			for i := 0; i < 50; i++ {
				p := randomProfile(rng)
				conf := domain.Know
				if i%2 == 1 {
					conf = domain.Assume
				}
				pets := []any{species}
				if i%3 == 0 {
					pets = append(pets, "rabbit")
				}
				p[domain.AttrPets] = domain.Attribute{Value: pets, Confidence: conf, Source: domain.SourceGiver}
				scent := []domain.Level{domain.LevelNone, domain.LevelLow, domain.LevelMedium, domain.LevelStrong}[rng.IntN(4)]
				f := domain.Flower{ID: 1, TradeName: genus, Color: "white", Scent: scent, Taxon: taxon, TaxonConfirmed: true}
				b := domain.Bouquet{ID: 1, Name: genus, Items: []domain.BouquetItem{{Flower: f, Stems: 1 + rng.IntN(30)}}}
				res := rules.CheckBouquet(b, p)
				require.Equal(t, domain.Block, res.Decision(), "%s (%s: %s) must be blocked", genus, species, tox)
				allowed, _ := rules.FilterCandidates([]domain.Bouquet{b}, p)
				require.Empty(t, allowed)
				checked++
			}
		}
	}
	require.Positive(t, checked)
}

// Неизвестная токсичность (нет ключа или unknown) для указанного питомца — тоже BLOCK (§7.1, п. 3).
func TestUnknownToxicityForListedPetBlocks(t *testing.T) {
	taxon := &domain.PlantTaxon{
		Genus: "Mysteria", Family: "Novaceae", Pollen: domain.LevelLow,
		PetToxicity: map[string]domain.Toxicity{"cat": domain.ToxNone},
	}
	f := domain.Flower{ID: 1, TradeName: "Загадка", Color: "white", Scent: domain.LevelNone, Taxon: taxon, TaxonConfirmed: true}
	b := domain.Bouquet{ID: 1, Name: "Загадка", Items: []domain.BouquetItem{{Flower: f, Stems: 3}}}
	for _, species := range []string{"dog", "horse"} {
		p := noRestrictions()
		p[domain.AttrPets] = domain.Attribute{Value: []any{species}, Confidence: domain.Know, Source: domain.SourceGiver}
		require.Equal(t, domain.Block, rules.CheckBouquet(b, p).Decision(), species)
	}
}

// Ни один случайный профиль не делает неподтверждённое растение допустимым (FR-C03).
func TestPropertyUnverifiedPlantAlwaysBlocked(t *testing.T) {
	taxa := loadTaxa(t)
	rng := rand.New(rand.NewPCG(1, 2))
	for genus, taxon := range taxa {
		for i := 0; i < 20; i++ {
			f := domain.Flower{ID: 1, TradeName: genus, Color: "white", Scent: domain.LevelNone, Taxon: taxon, TaxonConfirmed: false}
			b := domain.Bouquet{ID: 1, Name: genus, Items: []domain.BouquetItem{{Flower: f, Stems: 3}}}
			require.Equal(t, domain.Block, rules.CheckBouquet(b, randomProfile(rng)).Decision())
		}
	}
}
