package assistant

import (
	"errors"
	"slices"

	"github.com/zbekxzz/gulder/backend/internal/domain"
	"github.com/zbekxzz/gulder/backend/internal/recommend"
)

// Типы исключаемых позиций (POST /gift-sets/{id}/exclude).
const (
	ItemBouquet = "bouquet"
	ItemGift    = "gift"
)

// ExcludeRequest — исходный набор и позиция, которую даритель убирает (FR-G04).
type ExcludeRequest struct {
	BouquetID        int64
	GiftIDs          []int64
	Budget           domain.Money
	ExcludedBouquets []int64 // уже исключённые ранее в цепочке наборов
	ExcludedGifts    []int64
	ItemType         string
	ItemID           int64
}

// Ошибки исключения.
var (
	ErrItemNotInSet         = errors.New("item is not in the set")
	ErrNoBouquetReplacement = errors.New("no bouquet replacement within the budget")
)

// ExcludeResult — новый набор; Message — пояснение, если замену подобрать не удалось.
type ExcludeResult struct {
	Set              Set
	Message          string
	ExcludedBouquets []int64
	ExcludedGifts    []int64
}

// Exclude убирает позицию и подбирает замену в пределах budget − (сумма оставшихся) с повторной
// проверкой CheckGiftSet. Исключённая позиция больше не предлагается для этой цепочки наборов.
// Замена подбирается детерминированно (как fallback, §9.4): это дёшево и воспроизводимо.
func Exclude(c *recommend.Context, req ExcludeRequest) (ExcludeResult, error) {
	out := ExcludeResult{ExcludedBouquets: slices.Clone(req.ExcludedBouquets), ExcludedGifts: slices.Clone(req.ExcludedGifts)}
	switch req.ItemType {
	case ItemGift:
		if !slices.Contains(req.GiftIDs, req.ItemID) {
			return out, ErrItemNotInSet
		}
		out.ExcludedGifts = append(out.ExcludedGifts, req.ItemID)
	case ItemBouquet:
		if req.BouquetID != req.ItemID {
			return out, ErrItemNotInSet
		}
		out.ExcludedBouquets = append(out.ExcludedBouquets, req.ItemID)
	default:
		return out, ErrItemNotInSet
	}
	p := newPool(c, req.Budget, out.ExcludedBouquets, out.ExcludedGifts)

	// оставшиеся подарки (только те, что всё ещё допустимы и в наличии)
	var kept []domain.GiftItem
	for _, id := range req.GiftIDs {
		if id == req.ItemID && req.ItemType == ItemGift {
			continue
		}
		if g, ok := p.gByID[id]; ok {
			kept = append(kept, g.gift)
		}
	}
	keptSum := domain.Money(0)
	for _, g := range kept {
		keptSum += g.Price
	}

	bouquetID := req.BouquetID
	if req.ItemType == ItemBouquet {
		bouquetID = 0
		for _, s := range p.bouquets { // лучший по score букет, который помещается с оставшимися подарками
			if s.Bouquet.Price <= req.Budget-keptSum {
				if _, err := p.validate(ProposedSet{BouquetID: s.Bouquet.ID, GiftIDs: ids(kept)}, GeneratorFallback); err == nil {
					bouquetID = s.Bouquet.ID
					break
				}
			}
		}
		if bouquetID == 0 {
			return out, ErrNoBouquetReplacement
		}
	}
	b, ok := p.bByID[bouquetID]
	if !ok {
		return out, ErrNoBouquetReplacement
	}

	giftIDs := ids(kept)
	if req.ItemType == ItemGift {
		added := p.fillOne(b.Bouquet, kept, req.Budget-b.Bouquet.Price-keptSum)
		if added == nil {
			out.Message = "Замену в пределах оставшегося бюджета подобрать не удалось — набор без этой позиции"
		} else {
			giftIDs = append(giftIDs, added.ID)
		}
	}
	reasons := slices.Clone(b.Reasons)
	if len(reasons) > 3 {
		reasons = reasons[:3]
	}
	set, err := p.validate(ProposedSet{BouquetID: bouquetID, GiftIDs: giftIDs, Reasons: reasons}, GeneratorFallback)
	if err != nil {
		return out, err
	}
	out.Set = set
	return out, nil
}

// fillOne подбирает один подарок новой (для набора) категории в пределах left.
func (p *pool) fillOne(b domain.Bouquet, kept []domain.GiftItem, left domain.Money) *domain.GiftItem {
	added := p.fillGifts(b, kept, left)
	if len(added) == 0 {
		return nil
	}
	return &added[0]
}

func ids(gifts []domain.GiftItem) []int64 {
	out := make([]int64, 0, len(gifts))
	for _, g := range gifts {
		out = append(out, g.ID)
	}
	return out
}
