package recipients

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/zbekxzz/gulder/backend/internal/domain"
	"github.com/zbekxzz/gulder/backend/internal/store"
)

// Occasion — повод (FR-O01).
type Occasion struct {
	ID                int64        `json:"id"`
	RecipientID       int64        `json:"recipient_id"`
	Type              string       `json:"type"`
	Relationship      string       `json:"relationship,omitempty"`
	Date              *string      `json:"date"` // YYYY-MM-DD
	BudgetMin         domain.Money `json:"budget_min"`
	BudgetMax         domain.Money `json:"budget_max"`
	Place             string       `json:"place,omitempty"`
	Formality         *int         `json:"formality"`
	EmotionGoal       string       `json:"emotion_goal,omitempty"`
	NumerologyEnabled bool         `json:"numerology_enabled"`
	CreatedAt         time.Time    `json:"created_at"`
}

// Справочники повода (используются ranker'ом в occasion_fit, §8.1).
var (
	OccasionTypes = []string{"birthday", "anniversary", "march8", "valentine", "wedding", "apology", "gratitude", "congrats", "sympathy", "just_because"}
	Emotions      = []string{"joy", "gratitude", "romance", "support", "respect", "apology"}
	Places        = []string{"home", "office", "restaurant", "event", "hospital"}
	Relationships = []string{"partner", "spouse", "mother", "father", "relative", "friend", "colleague", "boss", "teacher", "other"}
)

// DateTime возвращает дату повода или ok=false.
func (o Occasion) DateTime() (time.Time, bool) {
	if o.Date == nil {
		return time.Time{}, false
	}
	t, err := time.Parse("2006-01-02", *o.Date)
	return t, err == nil
}

const occasionCols = `o.id, o.recipient_id, o.type, coalesce(o.relationship,''), to_char(o.date,'YYYY-MM-DD'),
	o.budget_min, o.budget_max, coalesce(o.place,''), o.formality, coalesce(o.emotion_goal,''), o.numerology_enabled, o.created_at`

func scanOccasion(row pgx.Row) (Occasion, error) {
	var o Occasion
	var formality *int16
	err := row.Scan(&o.ID, &o.RecipientID, &o.Type, &o.Relationship, &o.Date, &o.BudgetMin, &o.BudgetMax,
		&o.Place, &formality, &o.EmotionGoal, &o.NumerologyEnabled, &o.CreatedAt)
	if formality != nil {
		f := int(*formality)
		o.Formality = &f
	}
	return o, err
}

// CreateOccasion сохраняет повод получателя дарителя.
func CreateOccasion(ctx context.Context, q store.Querier, giverID int64, o Occasion) (Occasion, error) {
	row := q.QueryRow(ctx, `
		INSERT INTO occasions AS o (recipient_id, type, relationship, date, budget_min, budget_max, place, formality, emotion_goal, numerology_enabled)
		SELECT r.id, $3, NULLIF($4,''), $5::date, $6, $7, NULLIF($8,''), $9, NULLIF($10,''), $11
		  FROM recipients r WHERE r.id = $1 AND r.giver_id = $2
		RETURNING `+occasionCols,
		o.RecipientID, giverID, o.Type, o.Relationship, o.Date, o.BudgetMin, o.BudgetMax, o.Place, o.Formality, o.EmotionGoal, o.NumerologyEnabled)
	out, err := scanOccasion(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, store.ErrNotFound
	}
	if err != nil {
		return out, fmt.Errorf("insert occasion: %w", err)
	}
	return out, nil
}

// GetOccasion возвращает повод, если он принадлежит получателю дарителя.
func GetOccasion(ctx context.Context, q store.Querier, giverID, id int64) (Occasion, error) {
	row := q.QueryRow(ctx, `SELECT `+occasionCols+` FROM occasions o JOIN recipients r ON r.id = o.recipient_id
		WHERE o.id = $1 AND r.giver_id = $2`, id, giverID)
	o, err := scanOccasion(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return o, store.ErrNotFound
	}
	if err != nil {
		return o, fmt.Errorf("get occasion: %w", err)
	}
	return o, nil
}

// GetOccasionAny возвращает повод без проверки владельца (флорист при валидации замены).
func GetOccasionAny(ctx context.Context, q store.Querier, id int64) (Occasion, int64, error) {
	var giverID int64
	row := q.QueryRow(ctx, `SELECT `+occasionCols+`, r.giver_id FROM occasions o JOIN recipients r ON r.id = o.recipient_id WHERE o.id = $1`, id)
	var o Occasion
	var formality *int16
	err := row.Scan(&o.ID, &o.RecipientID, &o.Type, &o.Relationship, &o.Date, &o.BudgetMin, &o.BudgetMax,
		&o.Place, &formality, &o.EmotionGoal, &o.NumerologyEnabled, &o.CreatedAt, &giverID)
	if errors.Is(err, pgx.ErrNoRows) {
		return o, 0, store.ErrNotFound
	}
	if err != nil {
		return o, 0, fmt.Errorf("get occasion: %w", err)
	}
	if formality != nil {
		f := int(*formality)
		o.Formality = &f
	}
	return o, giverID, nil
}

// ListOccasions возвращает поводы получателя дарителя.
func ListOccasions(ctx context.Context, q store.Querier, giverID, recipientID int64) ([]Occasion, error) {
	rows, err := q.Query(ctx, `SELECT `+occasionCols+` FROM occasions o JOIN recipients r ON r.id = o.recipient_id
		WHERE o.recipient_id = $1 AND r.giver_id = $2 ORDER BY o.created_at DESC`, recipientID, giverID)
	if err != nil {
		return nil, fmt.Errorf("list occasions: %w", err)
	}
	defer rows.Close()
	var out []Occasion
	for rows.Next() {
		o, err := scanOccasion(rows)
		if err != nil {
			return nil, fmt.Errorf("scan occasion: %w", err)
		}
		out = append(out, o)
	}
	return out, rows.Err()
}
