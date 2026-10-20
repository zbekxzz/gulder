package rules

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/zbekxzz/gulder/backend/internal/domain"
)

// riskFamilies — семейства с частыми аллергиями на цветы (F3).
var riskFamilies = []string{"asteraceae", "liliaceae", "amaryllidaceae", "oleaceae"}

// riskGiftCategories — категории, где неизвестный состав при неизвестных аллергиях даёт WARN (G2).
var riskGiftCategories = []string{domain.GiftSweets, domain.GiftFragrance, domain.GiftCandle}

// allergenMatches: аллергия a совпадает с кодом позиции c, если c == a или c начинается с a+":" (§7.2).
func allergenMatches(allergy, code string) bool {
	return code == allergy || strings.HasPrefix(code, allergy+":")
}

// matchedCodes возвращает коды позиции, совпавшие с аллергиями получателя.
func matchedCodes(allergies, codes []string) []string {
	var out []string
	for _, c := range codes {
		for _, a := range allergies {
			if allergenMatches(a, c) && !slices.Contains(out, c) {
				out = append(out, c)
			}
		}
	}
	return out
}

// plantAllergenKeys: genus:<род>, family:<семейство>, AllergenCodes, pollen при пыльце ≥ medium.
func plantAllergenKeys(t *domain.PlantTaxon) []string {
	keys := []string{"genus:" + strings.ToLower(t.Genus), "family:" + strings.ToLower(t.Family)}
	for _, c := range t.AllergenCodes {
		keys = append(keys, strings.ToLower(c))
	}
	if t.Pollen.AtLeast(domain.LevelMedium) {
		keys = append(keys, "pollen")
	}
	return keys
}

func toxicityFor(t *domain.PlantTaxon, species string) domain.Toxicity {
	if tox, ok := t.PetToxicity[species]; ok && tox != "" {
		return tox
	}
	return domain.ToxUnknown
}

// checkFlower — правила F1–F8 для одного цветка букета (§7.3).
func checkFlower(res *Result, f domain.Flower, p domain.Profile) {
	item := f.TradeName

	// F1: неопознанное растение — BLOCK, дальше не проверяем (FR-C03).
	if f.Taxon == nil || !f.TaxonConfirmed {
		res.add(Violation{
			Code: CodeUnverifiedPlant, Decision: domain.Block, Item: item, Source: domain.SourceSystem,
			Message: fmt.Sprintf("Растение «%s» не подтверждено флористом: состав не проверен", item),
		})
		return
	}
	t := f.Taxon
	family := strings.ToLower(t.Family)

	// F2/F3: аллергии.
	if allergies, known := p.StringList(domain.AttrAllergies); known {
		if hit := matchedCodes(allergies, plantAllergenKeys(t)); len(hit) > 0 {
			res.add(Violation{
				Code: CodeAllergenMatch, Decision: domain.Block, Item: item,
				Attribute: domain.AttrAllergies, Source: p.SourceOf(domain.AttrAllergies),
				Message: fmt.Sprintf("«%s» содержит аллерген получателя: %s", item, strings.Join(hit, ", ")),
			})
		}
	} else if slices.Contains(riskFamilies, family) || t.Pollen.AtLeast(domain.LevelMedium) {
		res.add(Violation{
			Code: CodeAllergyUnknown, Decision: domain.Warn, Item: item, ConfidencePenalty: 0.15,
			Attribute: domain.AttrAllergies, Source: p.SourceOf(domain.AttrAllergies),
			Message: fmt.Sprintf("Неизвестно, есть ли у получателя аллергия; «%s» — частый аллерген", item),
		})
		res.missing(domain.AttrAllergies)
	}

	// F4/F5/F6: домашние животные.
	if pets, known := p.StringList(domain.AttrPets); known {
		src := p.SourceOf(domain.AttrPets)
		for _, species := range pets {
			if !slices.Contains(domain.PetSpecies, species) {
				res.add(Violation{
					Code: CodePetNoData, Decision: domain.Warn, Item: item, ConfidencePenalty: 0.20,
					Attribute: domain.AttrPets, Source: src,
					Message: fmt.Sprintf("Нет данных о токсичности «%s» для этого животного", item),
				})
				continue
			}
			if tox := toxicityFor(t, species); tox != domain.ToxNone {
				res.add(Violation{
					Code: CodePetToxic, Decision: domain.Block, Item: item,
					Attribute: domain.AttrPets, Source: src,
					Message: fmt.Sprintf("«%s» опасен для животного (%s: %s)", item, species, tox),
				})
			}
		}
	} else {
		cat, dog := toxicityFor(t, "cat"), toxicityFor(t, "dog")
		if cat != domain.ToxNone || dog != domain.ToxNone {
			penalty := 0.15
			if cat == domain.ToxSevere || cat == domain.ToxUnknown || dog == domain.ToxSevere || dog == domain.ToxUnknown {
				penalty = 0.35
			}
			res.add(Violation{
				Code: CodePetsUnknown, Decision: domain.Warn, Item: item, ConfidencePenalty: penalty,
				Attribute: domain.AttrPets, Source: p.SourceOf(domain.AttrPets),
				Message: fmt.Sprintf("Неизвестно, есть ли дома животные; «%s» опасен для кошек или собак", item),
			})
			res.missing(domain.AttrPets)
		}
	}

	// F7/F8: конфликтующие предпочтения — WARN без штрафа.
	if disliked, ok := p.StringList(domain.AttrDislikedFlowers); ok && slices.Contains(disliked, strings.ToLower(t.Genus)) {
		res.add(Violation{
			Code: CodeDislikedFlower, Decision: domain.Warn, Item: item,
			Attribute: domain.AttrDislikedFlowers, Source: p.SourceOf(domain.AttrDislikedFlowers),
			Message: fmt.Sprintf("Получатель не любит этот цветок: «%s»", item),
		})
	}
	if disliked, ok := p.StringList(domain.AttrDislikedColors); ok && slices.Contains(disliked, strings.ToLower(f.Color)) {
		res.add(Violation{
			Code: CodeDislikedColor, Decision: domain.Warn, Item: item,
			Attribute: domain.AttrDislikedColors, Source: p.SourceOf(domain.AttrDislikedColors),
			Message: fmt.Sprintf("Получатель не любит этот цвет: «%s»", item),
		})
	}
}

// checkScent — правила запаха и астмы S1–S7 (§7.4) для букета целиком или подарка.
func checkScent(res *Result, item string, scent, pollen domain.Level, p domain.Profile) {
	strongest := domain.MaxLevel(scent, pollen)

	asthma, asthmaKnown := p.Bool(domain.AttrAsthma)
	asthmaSrc := p.SourceOf(domain.AttrAsthma)
	switch {
	case asthmaKnown && asthma && strongest == domain.LevelStrong: // S1
		res.add(Violation{
			Code: CodeAsthmaTrigger, Decision: domain.Block, Item: item, Attribute: domain.AttrAsthma, Source: asthmaSrc,
			Message: fmt.Sprintf("«%s»: сильный запах или пыльца — риск при астме", item),
		})
	case asthmaKnown && asthma && strongest == domain.LevelMedium: // S2
		res.add(Violation{
			Code: CodeAsthmaCaution, Decision: domain.Warn, Item: item, Attribute: domain.AttrAsthma, Source: asthmaSrc,
			ConfidencePenalty: 0.10, Message: fmt.Sprintf("«%s»: заметный запах или пыльца — осторожно при астме", item),
		})
	case !asthmaKnown && strongest == domain.LevelStrong: // S3
		res.add(Violation{
			Code: CodeAsthmaUnknown, Decision: domain.Warn, Item: item, Attribute: domain.AttrAsthma, Source: asthmaSrc,
			ConfidencePenalty: 0.10, Message: fmt.Sprintf("Неизвестно, есть ли у получателя астма; у «%s» сильный запах или пыльца", item),
		})
		res.missing(domain.AttrAsthma)
	}

	sens, sensKnown := p.Level(domain.AttrScentSensitivity)
	sensSrc := p.SourceOf(domain.AttrScentSensitivity)
	tooStrong := func() {
		res.add(Violation{
			Code: CodeScentTooStrong, Decision: domain.Block, Item: item, Attribute: domain.AttrScentSensitivity, Source: sensSrc,
			Message: fmt.Sprintf("«%s»: запах слишком сильный для получателя", item),
		})
	}
	caution := func() {
		res.add(Violation{
			Code: CodeScentCaution, Decision: domain.Warn, Item: item, Attribute: domain.AttrScentSensitivity, Source: sensSrc,
			ConfidencePenalty: 0.05, Message: fmt.Sprintf("«%s»: заметный запах", item),
		})
	}
	switch {
	case !sensKnown:
		if scent == domain.LevelStrong { // S7
			res.add(Violation{
				Code: CodeScentUnknown, Decision: domain.Warn, Item: item, Attribute: domain.AttrScentSensitivity, Source: sensSrc,
				ConfidencePenalty: 0.10, Message: fmt.Sprintf("Неизвестно, как получатель относится к запахам; у «%s» сильный запах", item),
			})
			res.missing(domain.AttrScentSensitivity)
		}
	case sens == domain.LevelStrong: // S4
		if scent.AtLeast(domain.LevelMedium) {
			tooStrong()
		}
	case sens == domain.LevelMedium: // S5
		switch scent {
		case domain.LevelStrong:
			tooStrong()
		case domain.LevelMedium:
			caution()
		}
	case sens == domain.LevelLow: // S6
		if scent == domain.LevelStrong {
			caution()
		}
	}
}

// CheckBouquet проверяет состав букета (F1–F8) и запах букета целиком (S1–S7).
func CheckBouquet(b domain.Bouquet, p domain.Profile) Result {
	res := newResult(KindBouquet, b.ID)
	if len(b.Items) == 0 {
		res.add(Violation{
			Code: CodeEmptyComposition, Decision: domain.Block, Item: b.Name, Source: domain.SourceSystem,
			Message: "Состав букета пуст — проверить нечего",
		})
		return res
	}
	scent, pollen := domain.LevelNone, domain.LevelNone
	for _, it := range b.Items {
		checkFlower(&res, it.Flower, p)
		scent = domain.MaxLevel(scent, it.Flower.Scent)
		if it.Flower.Taxon != nil {
			pollen = domain.MaxLevel(pollen, it.Flower.Taxon.Pollen)
		}
	}
	checkScent(&res, b.Name, scent, pollen, p)
	return res
}

// CheckGift — правила подарка G1–G8 (§7.5), включая пищевые аллергены (FR-R01).
func CheckGift(g domain.GiftItem, p domain.Profile) Result {
	res := newResult(KindGift, g.ID)
	item := g.Name
	allergies, allergiesKnown := p.StringList(domain.AttrAllergies)
	allergySrc := p.SourceOf(domain.AttrAllergies)

	if g.AllergenCodes == nil {
		switch {
		case allergiesKnown && len(allergies) > 0: // G1
			res.add(Violation{
				Code: CodeUnknownComposition, Decision: domain.Block, Item: item, Attribute: domain.AttrAllergies, Source: allergySrc,
				Message: fmt.Sprintf("Состав «%s» неизвестен, а у получателя есть аллергии", item),
			})
		case !allergiesKnown && slices.Contains(riskGiftCategories, g.Category): // G2
			res.add(Violation{
				Code: CodeUnknownComposition, Decision: domain.Warn, Item: item, Attribute: domain.AttrAllergies, Source: allergySrc,
				ConfidencePenalty: 0.15, Message: fmt.Sprintf("Состав «%s» неизвестен, и неизвестно, есть ли у получателя аллергии", item),
			})
			res.missing(domain.AttrAllergies)
		}
	} else {
		codes := make([]string, 0, len(*g.AllergenCodes))
		for _, c := range *g.AllergenCodes {
			codes = append(codes, strings.ToLower(c))
		}
		switch {
		case allergiesKnown: // G3
			if hit := matchedCodes(allergies, codes); len(hit) > 0 {
				res.add(Violation{
					Code: CodeAllergenMatch, Decision: domain.Block, Item: item, Attribute: domain.AttrAllergies, Source: allergySrc,
					Message: fmt.Sprintf("«%s» содержит аллерген получателя: %s", item, strings.Join(hit, ", ")),
				})
			}
		case len(codes) > 0: // G4
			res.add(Violation{
				Code: CodeAllergyUnknown, Decision: domain.Warn, Item: item, Attribute: domain.AttrAllergies, Source: allergySrc,
				ConfidencePenalty: 0.15, Message: fmt.Sprintf("Неизвестно, есть ли у получателя аллергия; в «%s» есть аллергены", item),
			})
			res.missing(domain.AttrAllergies)
		}
	}

	pets, petsKnown := p.StringList(domain.AttrPets)
	petSrc := p.SourceOf(domain.AttrPets)
	switch {
	case petsKnown && len(pets) > 0 && g.PetHazard != nil && *g.PetHazard: // G5
		res.add(Violation{
			Code: CodePetHazard, Decision: domain.Block, Item: item, Attribute: domain.AttrPets, Source: petSrc,
			Message: fmt.Sprintf("«%s» опасен для домашних животных", item),
		})
	case petsKnown && len(pets) > 0 && g.PetHazard == nil: // G6
		res.add(Violation{
			Code: CodePetHazard, Decision: domain.Warn, Item: item, Attribute: domain.AttrPets, Source: petSrc,
			ConfidencePenalty: 0.15, Message: fmt.Sprintf("Нет данных, безопасен ли «%s» для животных", item),
		})
	case !petsKnown && g.PetHazard != nil && *g.PetHazard: // G7
		res.add(Violation{
			Code: CodePetsUnknown, Decision: domain.Warn, Item: item, Attribute: domain.AttrPets, Source: petSrc,
			ConfidencePenalty: 0.15, Message: fmt.Sprintf("Неизвестно, есть ли дома животные; «%s» опасен для них", item),
		})
		res.missing(domain.AttrPets)
	}

	// G8: запах подарка; парфюм всегда strong.
	scent := g.Scent
	if g.Category == domain.GiftFragrance {
		scent = domain.LevelStrong
	}
	if scent != domain.LevelNone {
		checkScent(&res, item, scent, domain.LevelNone, p)
	}
	return res
}

// Total — сумма набора по эффективным ценам.
func Total(b domain.Bouquet, gifts []domain.GiftItem) domain.Money {
	total := b.Price
	for _, g := range gifts {
		total += g.Price
	}
	return total
}

// CheckGiftSet объединяет проверки букета и подарков; сумма > бюджета → BLOCK OVER_BUDGET,
// повтор позиции → BLOCK DUPLICATE_ITEM (FR-G01, AC-4, AC-5).
func CheckGiftSet(b domain.Bouquet, gifts []domain.GiftItem, budget domain.Money, p domain.Profile) Result {
	res := newResult(KindGiftSet, b.ID)
	res.merge(CheckBouquet(b, p))
	seen := map[int64]bool{}
	for _, g := range gifts {
		if seen[g.ID] {
			res.add(Violation{
				Code: CodeDuplicateItem, Decision: domain.Block, Item: g.Name, Source: domain.SourceSystem,
				Message: fmt.Sprintf("«%s» повторяется в наборе", g.Name),
			})
			continue
		}
		seen[g.ID] = true
		res.merge(CheckGift(g, p))
	}
	if total := Total(b, gifts); total > budget {
		res.add(Violation{
			Code: CodeOverBudget, Decision: domain.Block, Source: domain.SourceSystem,
			Message: fmt.Sprintf("Сумма набора %s ₸ превышает бюджет %s ₸", total, budget),
		})
	}
	return res
}

// ErrFlowerNotInBouquet — заменяемого цветка нет в букете.
var ErrFlowerNotInBouquet = errors.New("flower is not in the bouquet")

// ValidateSubstitution строит новый состав (если заменяющий цветок уже есть — стебли суммируются)
// и прогоняет CheckBouquet (FR-S01, AC-3).
func ValidateSubstitution(b domain.Bouquet, fromFlowerID int64, to domain.Flower, p domain.Profile) (domain.Bouquet, Result, error) {
	idx := slices.IndexFunc(b.Items, func(it domain.BouquetItem) bool { return it.Flower.ID == fromFlowerID })
	if idx < 0 {
		return b, Result{}, ErrFlowerNotInBouquet
	}
	nb := b
	nb.Items = make([]domain.BouquetItem, 0, len(b.Items))
	stems := b.Items[idx].Stems
	merged := false
	for i, it := range b.Items {
		if i == idx {
			continue
		}
		if it.Flower.ID == to.ID {
			it.Stems += stems
			merged = true
		}
		nb.Items = append(nb.Items, it)
	}
	if !merged {
		// заменяющий цветок встаёт на место заменяемого
		nb.Items = slices.Insert(nb.Items, min(idx, len(nb.Items)), domain.BouquetItem{Flower: to, Stems: stems})
	}
	return nb, CheckBouquet(nb, p), nil
}

// Candidate — допущенный букет с результатом проверки.
type Candidate struct {
	Bouquet domain.Bouquet
	Result  Result
}

// FilterCandidates делит букеты на допущенные и исключённые. Вызывается до ranker (FR-R01).
func FilterCandidates(bouquets []domain.Bouquet, p domain.Profile) (allowed []Candidate, excluded []Result) {
	allowed, excluded = []Candidate{}, []Result{}
	for _, b := range bouquets {
		res := CheckBouquet(b, p)
		if res.Blocked() {
			excluded = append(excluded, res)
			continue
		}
		allowed = append(allowed, Candidate{Bouquet: b, Result: res})
	}
	return allowed, excluded
}
