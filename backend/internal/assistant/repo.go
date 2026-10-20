package assistant

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/zbekxzz/gulder/backend/internal/domain"
	"github.com/zbekxzz/gulder/backend/internal/store"
)

// Stored — сохранённый набор (gift_sets).
type Stored struct {
	ID               int64
	RunID            int64
	Budget           domain.Money
	BouquetID        int64
	GiftIDs          []int64
	ExcludedGifts    []int64
	ExcludedBouquets []int64
	ParentSetID      *int64
	Title            string
}

// Save пишет набор. CHECK (total_price <= budget) в БД — последняя линия защиты AC-4.
func Save(ctx context.Context, q store.Querier, runID int64, budget domain.Money, s Set, parent *Stored, excludedGifts, excludedBouquets []int64) (int64, error) {
	var parentID *int64
	if parent != nil {
		parentID = &parent.ID
	}
	var id int64
	err := q.QueryRow(ctx, `
		INSERT INTO gift_sets (run_id, budget, bouquet_id, gift_item_ids, total_price, reasons, warnings,
		                       excluded_item_ids, excluded_bouquet_ids, parent_set_id, generator, title)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,NULLIF($12,'')) RETURNING id`,
		runID, budget, s.Bouquet.ID, ids(s.Gifts), s.Total, nonNilStrings(s.Reasons), s.Warnings,
		nonNilIDs(excludedGifts), nonNilIDs(excludedBouquets), parentID, s.Generator, s.Title).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("save gift set: %w", err)
	}
	return id, nil
}

// LoadOwned загружает набор, если запуск принадлежит дарителю; иначе store.ErrNotFound.
func LoadOwned(ctx context.Context, q store.Querier, giverID, setID int64) (Stored, error) {
	var s Stored
	err := q.QueryRow(ctx, `
		SELECT gs.id, gs.run_id, gs.budget, gs.bouquet_id, gs.gift_item_ids, gs.excluded_item_ids, gs.excluded_bouquet_ids, gs.parent_set_id, coalesce(gs.title, '')
		  FROM gift_sets gs
		  JOIN recommendation_runs rr ON rr.id = gs.run_id
		  JOIN occasions o ON o.id = rr.occasion_id
		  JOIN recipients r ON r.id = o.recipient_id
		 WHERE gs.id = $1 AND r.giver_id = $2`, setID, giverID).
		Scan(&s.ID, &s.RunID, &s.Budget, &s.BouquetID, &s.GiftIDs, &s.ExcludedGifts, &s.ExcludedBouquets, &s.ParentSetID, &s.Title)
	if errors.Is(err, pgx.ErrNoRows) {
		return s, store.ErrNotFound
	}
	if err != nil {
		return s, fmt.Errorf("load gift set: %w", err)
	}
	return s, nil
}

func nonNilIDs(s []int64) []int64 {
	if s == nil {
		return []int64{}
	}
	return s
}

func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
