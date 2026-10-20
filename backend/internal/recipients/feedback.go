package recipients

import (
	"context"
	"fmt"
	"slices"

	"github.com/zbekxzz/gulder/backend/internal/domain"
	"github.com/zbekxzz/gulder/backend/internal/store"
)

// Feedback — отзыв дарителя о рекомендации (FR-F01).
type Feedback struct {
	RunID       int64
	RecipientID int64
	BouquetID   *int64
	Rating      *int
	Liked       *bool
	Comment     string
}

// FeedbackSignal — что известно о понравившемся/непонравившемся букете.
type FeedbackSignal struct {
	Genera []string // lowercase
	Colors []string
}

// SaveFeedback пишет отзыв и обновляет профиль только этого получателя (FR-F01, AC-8):
// liked → роды и цвета букета добавляются в liked_*, disliked → в disliked_*; source=feedback,
// confidence=assume. Поля, которые даритель или получатель указали сами, не перезаписываются.
func SaveFeedback(ctx context.Context, q store.Querier, fb Feedback, sig FeedbackSignal) error {
	if _, err := q.Exec(ctx, `INSERT INTO feedback (run_id, recipient_id, bouquet_id, rating, liked, comment)
		VALUES ($1,$2,$3,$4,$5,NULLIF($6,''))`, fb.RunID, fb.RecipientID, fb.BouquetID, fb.Rating, fb.Liked, fb.Comment); err != nil {
		return fmt.Errorf("insert feedback: %w", err)
	}
	if fb.Liked == nil || fb.BouquetID == nil {
		return nil
	}
	flowersKey, colorsKey := domain.AttrLikedFlowers, domain.AttrLikedColors
	if !*fb.Liked {
		flowersKey, colorsKey = domain.AttrDislikedFlowers, domain.AttrDislikedColors
	}
	existing, err := loadAttributes(ctx, q, fb.RecipientID)
	if err != nil {
		return err
	}
	var updates []Attribute
	for key, values := range map[string][]string{flowersKey: sig.Genera, colorsKey: sig.Colors} {
		if len(values) == 0 {
			continue
		}
		merged := []any{}
		skip := false
		for _, a := range existing {
			if a.Key != key {
				continue
			}
			// указанное дарителем или получателем точнее догадки по отзыву
			if a.Source != domain.SourceFeedback && a.Confidence != domain.Unknown {
				skip = true
			}
			if list, ok := a.Value.([]any); ok {
				merged = append(merged, list...)
			}
		}
		if skip {
			continue
		}
		for _, v := range values {
			if !slices.Contains(merged, any(v)) {
				merged = append(merged, v)
			}
		}
		updates = append(updates, Attribute{Key: key, Value: merged, Source: domain.SourceFeedback, Confidence: domain.Assume})
	}
	slices.SortFunc(updates, func(a, b Attribute) int { return compareStrings(a.Key, b.Key) })
	return UpsertAttributes(ctx, q, fb.RecipientID, updates, true)
}

func compareStrings(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}
