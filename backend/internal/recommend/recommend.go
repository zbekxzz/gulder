// Package recommend собирает рекомендацию: профиль получателя и повод → каталог →
// Rules Engine (до ranker'а, FR-R01) → ranker → карточки Top-5 через GiverView → запись запуска.
package recommend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/zbekxzz/gulder/backend/internal/catalog"
	"github.com/zbekxzz/gulder/backend/internal/domain"
	"github.com/zbekxzz/gulder/backend/internal/ranking"
	"github.com/zbekxzz/gulder/backend/internal/recipients"
	"github.com/zbekxzz/gulder/backend/internal/rules"
	"github.com/zbekxzz/gulder/backend/internal/store"
)

// TopN — размер выдачи (§8.2).
const TopN = 5

// Context — всё, что известно о запросе: повод, профиль, каталог и решение Rules Engine.
// Используется рекомендациями, ассистентом и фильтрами — все они опираются на один и тот же
// результат проверки ограничений (FR-L05).
type Context struct {
	Occasion   recipients.Occasion
	Recipient  recipients.Recipient
	Profile    domain.Profile
	Snapshot   catalog.Snapshot
	Allowed    []rules.Candidate
	Excluded   []rules.Result
	Scored     []ranking.Scored
	Numerology *ranking.NumerologyEntry
	NumVersion *int64
	Now        time.Time
}

// Build загружает данные и прогоняет Rules Engine и ranker для повода дарителя.
func Build(ctx context.Context, q store.Querier, cfg ranking.Config, giverID, occasionID int64, now time.Time) (*Context, error) {
	occ, err := recipients.GetOccasion(ctx, q, giverID, occasionID)
	if err != nil {
		return nil, err
	}
	rec, err := recipients.Get(ctx, q, giverID, occ.RecipientID)
	if err != nil {
		return nil, err
	}
	return BuildFor(ctx, q, cfg, occ, rec, now)
}

// BuildFor — то же для уже загруженных повода и получателя.
func BuildFor(ctx context.Context, q store.Querier, cfg ranking.Config, occ recipients.Occasion, rec recipients.Recipient, now time.Time) (*Context, error) {
	snap, err := catalog.LoadSnapshot(ctx, q, now)
	if err != nil {
		return nil, err
	}
	c := &Context{Occasion: occ, Recipient: rec, Profile: rec.Profile(), Snapshot: snap, Now: now}

	// FR-R01: ограничения применяются до ranking.
	c.Allowed, c.Excluded = rules.FilterCandidates(snap.Bouquets, c.Profile)

	// FR-N01–N07: нумерология только если включена на поводе и известна дата рождения.
	if occ.NumerologyEnabled {
		if birth, ok := ranking.BirthDate(c.Profile); ok {
			version, err := ranking.ActiveNumerology(ctx, q)
			if err != nil {
				return nil, err
			}
			if entry := version.Entry(ranking.NumerologyNumber(birth)); entry != nil {
				c.Numerology = entry
				c.NumVersion = &version.ID
			}
		}
	}
	c.Scored = cfg.Score(ranking.Input{Candidates: c.Allowed, Profile: c.Profile, Occasion: RankingOccasion(occ, now), Numerology: c.Numerology, Now: now})
	return c, nil
}

// RankingOccasion переводит повод в вход ranker'а.
func RankingOccasion(o recipients.Occasion, now time.Time) ranking.Occasion {
	ro := ranking.Occasion{
		Type: o.Type, Emotion: o.EmotionGoal, Place: o.Place, BudgetMin: o.BudgetMin, BudgetMax: o.BudgetMax,
		NumerologyEnabled: o.NumerologyEnabled,
	}
	if o.Formality != nil {
		ro.Formality = *o.Formality
	}
	if d, ok := o.DateTime(); ok {
		ro.Date = d
	} else {
		ro.Date = now
	}
	return ro
}

// ScoredByID ищет оценённого кандидата.
func (c *Context) ScoredByID(id int64) (ranking.Scored, bool) {
	for _, s := range c.Scored {
		if s.Bouquet.ID == id {
			return s, true
		}
	}
	return ranking.Scored{}, false
}

// ShopRef — магазин на карточке.
type ShopRef struct {
	ID        int64    `json:"id"`
	Name      string   `json:"name"`
	Address   string   `json:"address"`
	DistanceM *float64 `json:"distance_m"`
	OpenNow   bool     `json:"open_now"`
}

// CardItem — цветок в составе карточки (бейджи токсичности и запаха — данные каталога,
// не ограничения получателя).
type CardItem struct {
	FlowerID     int64    `json:"flower_id"`
	TradeName    string   `json:"trade_name"`
	Color        string   `json:"color"`
	Stems        int      `json:"stems"`
	Genus        string   `json:"genus"`
	Scent        string   `json:"scent"`
	ToxicFor     []string `json:"toxic_for"`
	ImageURL     string   `json:"image_url,omitempty"`
	ImageLicense string   `json:"image_license,omitempty"`
	ImageAuthor  string   `json:"image_author,omitempty"`
}

// NumerologyCard — нумерология на карточке (FR-N05).
type NumerologyCard struct {
	Number  int    `json:"number"`
	Meaning string `json:"meaning"`
	Label   string `json:"label"`
}

// Card — карточка Top-5 (§8.2).
type Card struct {
	BouquetID         int64               `json:"bouquet_id"`
	Rank              int                 `json:"rank"`
	Name              string              `json:"name"`
	Style             string              `json:"style"`
	CompositionFormat string              `json:"composition_format"`
	ImageURL          string              `json:"image_url,omitempty"`
	Score             float64             `json:"score"`
	Confidence        float64             `json:"confidence"`
	Reasons           []string            `json:"reasons"`
	Conflicts         []string            `json:"conflicts"`
	MissingInfo       []string            `json:"missing_info"`
	Warnings          []rules.GiverReason `json:"warnings"`
	Numerology        *NumerologyCard     `json:"numerology"`
	Shop              ShopRef             `json:"shop"`
	Price             domain.Money        `json:"price"`
	DiscountPrice     *domain.Money       `json:"discount_price"`
	DiscountUntil     *time.Time          `json:"discount_until"`
	InStock           bool                `json:"in_stock"`
	Items             []CardItem          `json:"items"`
}

func round3(x float64) float64 { return float64(int64(x*1000+0.5)) / 1000 }

// NewCard строит карточку через GiverView: скрытые ограничения получателя не раскрываются.
func (c *Context) NewCard(s ranking.Scored, rank int) Card {
	b := s.Bouquet
	card := Card{
		BouquetID: b.ID, Rank: rank, Name: b.Name, Style: b.Style, CompositionFormat: b.CompositionFormat, ImageURL: b.ImageURL,
		Score: round3(s.Score), Confidence: round3(s.Confidence),
		Reasons: s.Reasons, Conflicts: s.Conflicts,
		MissingInfo: s.Result.GiverView().MissingInfo, Warnings: s.Result.GiverView().Reasons,
		Price: b.Offer.Price, InStock: b.InStock, Items: []CardItem{},
	}
	if b.Offer.DiscountActive(c.Now) {
		card.DiscountPrice, card.DiscountUntil = b.Offer.DiscountPrice, b.Offer.DiscountUntil
	}
	if shop, ok := c.Snapshot.Shops[b.ShopID]; ok {
		card.Shop = ShopRef{ID: shop.ID, Name: shop.Name, Address: shop.Address, OpenNow: shop.OpeningHours.OpenAt(c.Now, shop.Location())}
	}
	if c.Numerology != nil {
		card.Numerology = &NumerologyCard{Number: c.Numerology.Number, Meaning: c.Numerology.Meaning, Label: ranking.NumerologyLabel}
	}
	for _, it := range b.Items {
		ci := CardItem{
			FlowerID: it.Flower.ID, TradeName: it.Flower.TradeName, Color: it.Flower.Color, Stems: it.Stems,
			Scent: string(it.Flower.Scent), ToxicFor: []string{},
		}
		if t := it.Flower.Taxon; t != nil {
			ci.Genus = t.Genus
			ci.ImageURL, ci.ImageLicense, ci.ImageAuthor = t.ImageURL, t.ImageLicense, t.ImageAuthor
			for _, sp := range domain.PetSpecies {
				if tox := t.PetToxicity[sp]; tox != domain.ToxNone {
					ci.ToxicFor = append(ci.ToxicFor, sp)
				}
			}
		}
		card.Items = append(card.Items, ci)
	}
	return card
}

// Question — уточняющий вопрос в ответе (FR-R04).
type Question struct {
	Attribute string `json:"attribute"`
	Text      string `json:"text"`
}

// Run — сохранённый запуск рекомендаций.
type Run struct {
	RunID              int64          `json:"run_id"`
	OccasionID         int64          `json:"occasion_id"`
	RecipientID        int64          `json:"recipient_id"`
	ModelVersion       string         `json:"model_version"`
	RulesVersion       string         `json:"rules_version"`
	NumerologyUsed     bool           `json:"numerology_used"`
	Filters            map[string]any `json:"filters"`
	Items              []Card         `json:"items"`
	ClarifyingQuestion *Question      `json:"clarifying_question"`
	ExcludedCount      int            `json:"excluded_count"`
	CreatedAt          time.Time      `json:"created_at"`
}

type excludedEntry struct {
	BouquetID int64    `json:"bouquet_id"`
	Codes     []string `json:"codes"`
}

// Recommend строит Top-5 и сохраняет запуск в одной транзакции (§18.2).
func Recommend(ctx context.Context, st *store.Store, cfg ranking.Config, giverID, occasionID int64, filters map[string]any, now time.Time) (Run, error) {
	var run Run
	err := st.InTx(ctx, func(tx pgx.Tx) error {
		c, err := Build(ctx, tx, cfg, giverID, occasionID, now)
		if err != nil {
			return err
		}
		run = Run{
			OccasionID: occasionID, RecipientID: c.Recipient.ID, ModelVersion: cfg.ModelVersion, RulesVersion: rules.Version,
			NumerologyUsed: c.Numerology != nil, Filters: filters, Items: []Card{}, ExcludedCount: len(c.Excluded),
		}
		if run.Filters == nil {
			run.Filters = map[string]any{}
		}
		for i, s := range ranking.SelectTop(c.Scored, TopN) {
			run.Items = append(run.Items, c.NewCard(s, i+1))
		}
		results := make([]rules.Result, 0, len(c.Allowed))
		for _, a := range c.Allowed {
			results = append(results, a.Result)
		}
		if q := rules.BestClarifyingQuestion(results); q != nil {
			run.ClarifyingQuestion = &Question{Attribute: q.Attribute, Text: q.Text}
		}
		excluded := make([]excludedEntry, 0, len(c.Excluded))
		for _, r := range c.Excluded {
			excluded = append(excluded, excludedEntry{BouquetID: r.ItemID, Codes: r.Codes()})
		}
		var questionText *string
		if run.ClarifyingQuestion != nil {
			questionText = &run.ClarifyingQuestion.Text
		}
		return tx.QueryRow(ctx, `
			INSERT INTO recommendation_runs (occasion_id, model_version, rules_version, numerology_used, numerology_version_id,
			                                 filters, results, excluded, clarifying_question)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id, created_at`,
			occasionID, run.ModelVersion, run.RulesVersion, run.NumerologyUsed, c.NumVersion,
			run.Filters, run.Items, excluded, questionText).Scan(&run.RunID, &run.CreatedAt)
	})
	if err != nil {
		return Run{}, fmt.Errorf("recommend: %w", err)
	}
	return run, nil
}

// RunOwner возвращает повод и получателя запуска, если он принадлежит дарителю.
func RunOwner(ctx context.Context, q store.Querier, giverID, runID int64) (occasionID, recipientID int64, err error) {
	err = q.QueryRow(ctx, `SELECT o.id, r.id FROM recommendation_runs rr
		JOIN occasions o ON o.id = rr.occasion_id JOIN recipients r ON r.id = o.recipient_id
		WHERE rr.id = $1 AND r.giver_id = $2`, runID, giverID).Scan(&occasionID, &recipientID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, 0, store.ErrNotFound
	}
	if err != nil {
		return 0, 0, fmt.Errorf("run owner: %w", err)
	}
	return occasionID, recipientID, nil
}

const runCols = `rr.id, rr.occasion_id, o.recipient_id, rr.model_version, rr.rules_version, rr.numerology_used,
	rr.filters, rr.results, jsonb_array_length(rr.excluded), rr.clarifying_question, rr.created_at`

func scanRun(row pgx.Row) (Run, error) {
	var r Run
	var filters, results []byte
	var question *string
	if err := row.Scan(&r.RunID, &r.OccasionID, &r.RecipientID, &r.ModelVersion, &r.RulesVersion, &r.NumerologyUsed,
		&filters, &results, &r.ExcludedCount, &question, &r.CreatedAt); err != nil {
		return r, err //nolint:wrapcheck // вызывающий оборачивает
	}
	if err := json.Unmarshal(filters, &r.Filters); err != nil {
		return r, fmt.Errorf("decode filters: %w", err)
	}
	if err := json.Unmarshal(results, &r.Items); err != nil {
		return r, fmt.Errorf("decode results: %w", err)
	}
	if question != nil {
		r.ClarifyingQuestion = &Question{Attribute: questionAttribute(*question), Text: *question}
	}
	return r, nil
}

func questionAttribute(text string) string {
	for _, attr := range []string{domain.AttrAllergies, domain.AttrPets, domain.AttrAsthma, domain.AttrScentSensitivity} {
		if rules.QuestionText(attr) == text {
			return attr
		}
	}
	return ""
}

// GetRun возвращает сохранённый запуск дарителя.
func GetRun(ctx context.Context, q store.Querier, giverID, runID int64) (Run, error) {
	r, err := scanRun(q.QueryRow(ctx, `SELECT `+runCols+` FROM recommendation_runs rr
		JOIN occasions o ON o.id = rr.occasion_id JOIN recipients rc ON rc.id = o.recipient_id
		WHERE rr.id = $1 AND rc.giver_id = $2`, runID, giverID))
	if errors.Is(err, pgx.ErrNoRows) {
		return r, store.ErrNotFound
	}
	if err != nil {
		return r, fmt.Errorf("get run: %w", err)
	}
	return r, nil
}

// ListRuns — история рекомендаций получателя (новые первыми).
func ListRuns(ctx context.Context, q store.Querier, giverID, recipientID int64) ([]Run, error) {
	rows, err := q.Query(ctx, `SELECT `+runCols+` FROM recommendation_runs rr
		JOIN occasions o ON o.id = rr.occasion_id JOIN recipients rc ON rc.id = o.recipient_id
		WHERE o.recipient_id = $1 AND rc.giver_id = $2 ORDER BY rr.created_at DESC, rr.id DESC LIMIT 50`, recipientID, giverID)
	if err != nil {
		return nil, fmt.Errorf("list runs: %w", err)
	}
	defer rows.Close()
	out := []Run{}
	for rows.Next() {
		r, err := scanRun(rows)
		if err != nil {
			return nil, fmt.Errorf("scan run: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// FilterKeys — допустимые фильтры запуска (§10).
var FilterKeys = []string{"nearby", "cheaper", "discount"}

// NormalizeFilters оставляет только известные булевы фильтры.
func NormalizeFilters(in map[string]bool) (map[string]any, error) {
	out := map[string]any{}
	for k, v := range in {
		if !slices.Contains(FilterKeys, k) {
			return nil, fmt.Errorf("unknown filter %q (allowed: %s)", k, strings.Join(FilterKeys, ", "))
		}
		out[k] = v
	}
	return out, nil
}
