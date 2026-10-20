package httpapi

import (
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/zbekxzz/gulder/backend/internal/assistant"
	"github.com/zbekxzz/gulder/backend/internal/domain"
	"github.com/zbekxzz/gulder/backend/internal/recommend"
	"github.com/zbekxzz/gulder/backend/internal/rules"
)

type giftDTO struct {
	ID            int64         `json:"id"`
	Name          string        `json:"name"`
	Category      string        `json:"category"`
	ShopID        int64         `json:"shop_id"`
	Price         domain.Money  `json:"price"`
	DiscountPrice *domain.Money `json:"discount_price"`
	DiscountUntil *time.Time    `json:"discount_until"`
}

type giftSetDTO struct {
	ID                 int64               `json:"id"`
	RunID              int64               `json:"run_id"`
	ParentSetID        *int64              `json:"parent_set_id"`
	Title              string              `json:"title"`
	Generator          string              `json:"generator"`
	Budget             domain.Money        `json:"budget"`
	TotalPrice         domain.Money        `json:"total_price"`
	BudgetLeft         domain.Money        `json:"budget_left"`
	Bouquet            recommend.Card      `json:"bouquet"`
	Gifts              []giftDTO           `json:"gifts"`
	Reasons            []string            `json:"reasons"`
	Warnings           []rules.GiverReason `json:"warnings"`
	MissingInfo        []string            `json:"missing_info"`
	ExcludedGiftIDs    []int64             `json:"excluded_gift_ids"`
	ExcludedBouquetIDs []int64             `json:"excluded_bouquet_ids"`
}

func toGiftSetDTO(c *recommend.Context, id, runID int64, parent *int64, budget domain.Money, s assistant.Set, exGifts, exBouquets []int64) giftSetDTO {
	scored, _ := c.ScoredByID(s.Bouquet.ID)
	dto := giftSetDTO{
		ID: id, RunID: runID, ParentSetID: parent, Title: s.Title, Generator: s.Generator, Budget: budget,
		TotalPrice: s.Total, BudgetLeft: s.BudgetLeft, Bouquet: c.NewCard(scored, 0), Gifts: []giftDTO{},
		Reasons: nonNilSlice(s.Reasons), Warnings: nonNilSlice(s.Warnings), MissingInfo: nonNilSlice(s.MissingInfo),
		ExcludedGiftIDs: nonNilSlice(exGifts), ExcludedBouquetIDs: nonNilSlice(exBouquets),
	}
	for _, g := range s.Gifts {
		gd := giftDTO{ID: g.ID, Name: g.Name, Category: g.Category, ShopID: g.ShopID, Price: g.Offer.Price}
		if g.Offer.DiscountActive(c.Now) {
			gd.DiscountPrice, gd.DiscountUntil = g.Offer.DiscountPrice, g.Offer.DiscountUntil
		}
		dto.Gifts = append(dto.Gifts, gd)
	}
	return dto
}

type giftSetsRequest struct {
	RunID  int64        `json:"run_id" validate:"required,gt=0"`
	Budget domain.Money `json:"budget" validate:"gt=0"`
}

// createGiftSets — POST /gift-sets: 2–3 набора «букет + подарки» в пределах бюджета (FR-G01–G03).
func (s *Server) createGiftSets(w http.ResponseWriter, r *http.Request) {
	var req giftSetsRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	if err := validateStruct(req); err != nil {
		writeError(w, r, err)
		return
	}
	giver := principal(r).UserID
	now := s.now()
	occasionID, _, err := recommend.RunOwner(r.Context(), s.store.Pool, giver, req.RunID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	c, err := recommend.Build(r.Context(), s.store.Pool, s.ranking, giver, occasionID, now)
	if err != nil {
		writeError(w, r, err)
		return
	}
	// LLM-вызов — вне транзакции: БД не держится открытой на время запроса к модели
	res, err := assistant.Assemble(r.Context(), s.generator, c, req.Budget)
	if errors.Is(err, assistant.ErrNoCandidates) {
		writeJSON(w, http.StatusOK, map[string]any{
			"sets": []giftSetDTO{}, "generator": assistant.GeneratorFallback,
			"message": "В этот бюджет не помещается ни один подходящий букет",
		})
		return
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	out := make([]giftSetDTO, 0, len(res.Sets))
	err = s.store.InTx(r.Context(), func(tx pgx.Tx) error {
		for _, set := range res.Sets {
			id, err := assistant.Save(r.Context(), tx, req.RunID, req.Budget, set, nil, nil, nil)
			if err != nil {
				return err
			}
			out = append(out, toGiftSetDTO(c, id, req.RunID, nil, req.Budget, set, nil, nil))
		}
		return nil
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	body := map[string]any{"sets": out, "generator": res.Generator}
	if len(out) < assistant.MinSets {
		body["message"] = "Удалось собрать меньше двух наборов в этом бюджете"
	}
	writeJSON(w, http.StatusCreated, body)
}

type excludeRequest struct {
	ItemType string `json:"item_type" validate:"required,oneof=bouquet gift"`
	ItemID   int64  `json:"item_id" validate:"required,gt=0"`
}

// excludeGiftSetItem — POST /gift-sets/{id}/exclude: новый набор без позиции с заменой (FR-G04).
func (s *Server) excludeGiftSetItem(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, err)
		return
	}
	var req excludeRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, err)
		return
	}
	if err := validateStruct(req); err != nil {
		writeError(w, r, err)
		return
	}
	giver := principal(r).UserID
	parent, err := assistant.LoadOwned(r.Context(), s.store.Pool, giver, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	occasionID, _, err := recommend.RunOwner(r.Context(), s.store.Pool, giver, parent.RunID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	c, err := recommend.Build(r.Context(), s.store.Pool, s.ranking, giver, occasionID, s.now())
	if err != nil {
		writeError(w, r, err)
		return
	}
	res, err := assistant.Exclude(c, assistant.ExcludeRequest{
		BouquetID: parent.BouquetID, GiftIDs: parent.GiftIDs, Budget: parent.Budget,
		ExcludedBouquets: parent.ExcludedBouquets, ExcludedGifts: parent.ExcludedGifts,
		ItemType: req.ItemType, ItemID: req.ItemID,
	})
	switch {
	case errors.Is(err, assistant.ErrItemNotInSet):
		writeError(w, r, errValidation(map[string]any{"item_id": "позиции нет в наборе"}))
		return
	case errors.Is(err, assistant.ErrNoBouquetReplacement):
		writeError(w, r, errConflict("no_replacement", "Не нашлось другого подходящего букета в пределах бюджета", nil))
		return
	case err != nil:
		writeError(w, r, err)
		return
	}
	var newID int64
	err = s.store.InTx(r.Context(), func(tx pgx.Tx) error {
		var err error
		newID, err = assistant.Save(r.Context(), tx, parent.RunID, parent.Budget, res.Set, &parent, res.ExcludedGifts, res.ExcludedBouquets)
		return err
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	body := map[string]any{"set": toGiftSetDTO(c, newID, parent.RunID, &parent.ID, parent.Budget, res.Set, res.ExcludedGifts, res.ExcludedBouquets)}
	if res.Message != "" {
		body["message"] = res.Message
	}
	writeJSON(w, http.StatusCreated, body)
}
