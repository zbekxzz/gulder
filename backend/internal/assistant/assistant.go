// Package assistant — AI-ассистент по бюджету (§9, FR-G01–G04): 2–3 набора «букет + подарки».
//
// LLM видит только допущенные Rules Engine позиции каталога (без персональных данных получателя)
// и отвечает структурированным JSON с ID. Код проверяет каждый набор заново: ID из списка
// кандидатов, CheckGiftSet ≠ BLOCK, сумма ≤ бюджета. Цены, остаток и предупреждения считает код.
// Без ключа LLM работает детерминированный fallback (§9.4).
package assistant

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"

	"github.com/zbekxzz/gulder/backend/internal/domain"
	"github.com/zbekxzz/gulder/backend/internal/ranking"
	"github.com/zbekxzz/gulder/backend/internal/recommend"
	"github.com/zbekxzz/gulder/backend/internal/rules"
)

// Источник набора (§9.4).
const (
	GeneratorLLM      = "llm"
	GeneratorFallback = "fallback"
)

// Лимиты кандидатов (§9.1).
const (
	MaxBouquetCandidates = 15
	MaxGiftCandidates    = 30
	MinSets              = 2
	MaxSets              = 3
)

// ProposedSet — набор, предложенный генератором (LLM или fallback).
type ProposedSet struct {
	BouquetID int64    `json:"bouquet_id"`
	GiftIDs   []int64  `json:"gift_ids"`
	Reasons   []string `json:"reasons"`
	Title     string   `json:"title"`
}

// Proposal — ответ генератора.
type Proposal struct {
	Sets []ProposedSet `json:"sets"`
}

// BouquetCandidate — букет, который можно показать LLM.
type BouquetCandidate struct {
	ID       int64        `json:"id"`
	Name     string       `json:"name"`
	Price    domain.Money `json:"price"`
	Style    string       `json:"style"`
	Format   string       `json:"format"`
	Flowers  []string     `json:"flowers"`
	Reasons  []string     `json:"reasons"`
	Warnings []string     `json:"warnings"`
	Score    float64      `json:"score"`
}

// GiftCandidate — подарок, который можно показать LLM.
type GiftCandidate struct {
	ID        int64        `json:"id"`
	Name      string       `json:"name"`
	Category  string       `json:"category"`
	Price     domain.Money `json:"price"`
	Warnings  []string     `json:"warnings"`
	Relevance float64      `json:"relevance"`
}

// Prompt — всё, что получает генератор. Персональных данных получателя здесь нет: ограничения
// уже применены, предупреждения прошли через GiverView (§9.1, п. 2).
type Prompt struct {
	OccasionType string             `json:"occasion_type"`
	Emotion      string             `json:"emotion,omitempty"`
	Budget       domain.Money       `json:"budget"`
	Bouquets     []BouquetCandidate `json:"bouquets"`
	Gifts        []GiftCandidate    `json:"gifts"`
	// Errors — ошибки предыдущей попытки для повторного запроса (§9.1, п. 5).
	Errors []string `json:"errors,omitempty"`
}

// Generator предлагает наборы. Реализации: Claude (LLM) и тестовые моки.
type Generator interface {
	Propose(ctx context.Context, p Prompt) (Proposal, error)
}

// Set — проверенный набор (FR-G03): состав, цена, остаток, причины, предупреждения.
type Set struct {
	Title       string
	Bouquet     domain.Bouquet
	Gifts       []domain.GiftItem
	Total       domain.Money
	BudgetLeft  domain.Money
	Reasons     []string
	Warnings    []rules.GiverReason
	MissingInfo []string
	Generator   string
}

// Result — итог работы ассистента.
type Result struct {
	Sets      []Set
	Generator string // llm, если хотя бы один набор от LLM; иначе fallback
	Rejected  []string
}

// pool — кандидаты и доступ к ним по ID.
type pool struct {
	ctx      *recommend.Context
	budget   domain.Money
	bouquets []ranking.Scored
	gifts    []scoredGift
	bByID    map[int64]ranking.Scored
	gByID    map[int64]scoredGift
}

type scoredGift struct {
	gift      domain.GiftItem
	result    rules.Result
	relevance float64
}

// categoryRelevance — насколько категория уместна к поводу (по умолчанию — базовое значение).
var categoryRelevance = map[string]float64{
	domain.GiftCard: 1.0, domain.GiftSweets: 0.8, domain.GiftJewelry: 0.7, domain.GiftSoftToy: 0.6,
	domain.GiftCandle: 0.6, domain.GiftFragrance: 0.5, domain.GiftOther: 0.4,
}

var occasionBoost = map[string][]string{
	"valentine":   {domain.GiftJewelry, domain.GiftSoftToy},
	"anniversary": {domain.GiftJewelry, domain.GiftFragrance},
	"birthday":    {domain.GiftSweets, domain.GiftSoftToy},
	"march8":      {domain.GiftSweets, domain.GiftCandle},
	"gratitude":   {domain.GiftSweets},
	"sympathy":    {domain.GiftCandle},
}

func giftRelevance(g domain.GiftItem, occasionType string, r rules.Result) float64 {
	rel := categoryRelevance[g.Category]
	if slices.Contains(occasionBoost[occasionType], g.Category) {
		rel += 0.3
	}
	return rel * r.ConfidenceMultiplier()
}

// newPool отбирает кандидатов (§9.1, п. 1): букеты — допущенные, в наличии, по цене в бюджете,
// Top-15 по score; подарки — CheckGift ≠ BLOCK, в наличии, цена ≤ бюджета, до 30 по релевантности.
func newPool(c *recommend.Context, budget domain.Money, excludedBouquets, excludedGifts []int64) *pool {
	p := &pool{ctx: c, budget: budget, bByID: map[int64]ranking.Scored{}, gByID: map[int64]scoredGift{}}
	for _, s := range c.Scored {
		if len(p.bouquets) == MaxBouquetCandidates {
			break
		}
		if !s.Bouquet.InStock || s.Bouquet.Price > budget || slices.Contains(excludedBouquets, s.Bouquet.ID) {
			continue
		}
		p.bouquets = append(p.bouquets, s)
		p.bByID[s.Bouquet.ID] = s
	}
	var gifts []scoredGift
	for _, g := range c.Snapshot.Gifts {
		if !g.InStock || g.Price > budget || slices.Contains(excludedGifts, g.ID) {
			continue
		}
		res := rules.CheckGift(g, c.Profile)
		if res.Blocked() { // FR-R01: ограничения применяются и к подаркам, включая пищевые аллергены
			continue
		}
		gifts = append(gifts, scoredGift{gift: g, result: res, relevance: giftRelevance(g, c.Occasion.Type, res)})
	}
	slices.SortStableFunc(gifts, func(a, b scoredGift) int {
		if d := cmp.Compare(b.relevance, a.relevance); d != 0 {
			return d
		}
		return cmp.Compare(a.gift.ID, b.gift.ID)
	})
	if len(gifts) > MaxGiftCandidates {
		gifts = gifts[:MaxGiftCandidates]
	}
	p.gifts = gifts
	for _, g := range gifts {
		p.gByID[g.gift.ID] = g
	}
	return p
}

func messages(rs []rules.GiverReason) []string {
	out := make([]string, 0, len(rs))
	for _, r := range rs {
		out = append(out, r.Message)
	}
	return out
}

func (p *pool) prompt() Prompt {
	pr := Prompt{
		OccasionType: p.ctx.Occasion.Type, Emotion: p.ctx.Occasion.EmotionGoal, Budget: p.budget,
		Bouquets: []BouquetCandidate{}, Gifts: []GiftCandidate{},
	}
	for _, s := range p.bouquets {
		var flowers []string
		for _, it := range s.Bouquet.Items {
			flowers = append(flowers, fmt.Sprintf("%s × %d", it.Flower.TradeName, it.Stems))
		}
		pr.Bouquets = append(pr.Bouquets, BouquetCandidate{
			ID: s.Bouquet.ID, Name: s.Bouquet.Name, Price: s.Bouquet.Price, Style: s.Bouquet.Style, Format: s.Bouquet.CompositionFormat,
			Flowers: flowers, Reasons: s.Reasons, Warnings: messages(s.Result.GiverView().Reasons), Score: float64(int(s.Score*100)) / 100,
		})
	}
	for _, g := range p.gifts {
		pr.Gifts = append(pr.Gifts, GiftCandidate{
			ID: g.gift.ID, Name: g.gift.Name, Category: g.gift.Category, Price: g.gift.Price,
			Warnings: messages(g.result.GiverView().Reasons), Relevance: float64(int(g.relevance*100)) / 100,
		})
	}
	return pr
}

// validate проверяет предложенный набор кодом (§9.1, п. 4). Позиции не из списка кандидатов —
// набор отбрасывается (FR-G02); BLOCK или превышение бюджета — тоже (AC-4, AC-5).
func (p *pool) validate(ps ProposedSet, generator string) (Set, error) {
	b, ok := p.bByID[ps.BouquetID]
	if !ok {
		return Set{}, fmt.Errorf("букет %d не из списка кандидатов", ps.BouquetID)
	}
	gifts := make([]domain.GiftItem, 0, len(ps.GiftIDs))
	for _, id := range ps.GiftIDs {
		g, ok := p.gByID[id]
		if !ok {
			return Set{}, fmt.Errorf("подарок %d не из списка кандидатов", id)
		}
		gifts = append(gifts, g.gift)
	}
	res := rules.CheckGiftSet(b.Bouquet, gifts, p.budget, p.ctx.Profile)
	if res.Blocked() {
		return Set{}, fmt.Errorf("набор с букетом %d отклонён правилами: %v", ps.BouquetID, res.Codes())
	}
	total := rules.Total(b.Bouquet, gifts)
	gv := res.GiverView()
	reasons := ps.Reasons
	if len(reasons) == 0 {
		reasons = b.Reasons
	}
	return Set{
		Title: ps.Title, Bouquet: b.Bouquet, Gifts: gifts, Total: total, BudgetLeft: p.budget - total,
		Reasons: reasons, Warnings: gv.Reasons, MissingInfo: gv.MissingInfo, Generator: generator,
	}, nil
}

// distinct: наборы отличаются букетом или минимум двумя подарками (§9.3).
func distinct(a, b Set) bool {
	if a.Bouquet.ID != b.Bouquet.ID {
		return true
	}
	diff := 0
	for _, g := range a.Gifts {
		if !slices.ContainsFunc(b.Gifts, func(x domain.GiftItem) bool { return x.ID == g.ID }) {
			diff++
		}
	}
	for _, g := range b.Gifts {
		if !slices.ContainsFunc(a.Gifts, func(x domain.GiftItem) bool { return x.ID == g.ID }) {
			diff++
		}
	}
	return diff >= 2
}

func (r *Result) accept(s Set) bool {
	if len(r.Sets) >= MaxSets {
		return false
	}
	for _, existing := range r.Sets {
		if !distinct(existing, s) {
			return false
		}
	}
	r.Sets = append(r.Sets, s)
	return true
}

// ErrNoCandidates — в бюджет не помещается ни один допущенный букет.
var ErrNoCandidates = errors.New("no bouquet fits the budget")

// Assemble строит 2–3 набора (FR-G01). gen == nil — только детерминированный fallback.
func Assemble(ctx context.Context, gen Generator, c *recommend.Context, budget domain.Money) (Result, error) {
	p := newPool(c, budget, nil, nil)
	if len(p.bouquets) == 0 {
		return Result{}, ErrNoCandidates
	}
	res := Result{Generator: GeneratorFallback}
	if gen != nil {
		prompt := p.prompt()
		for attempt := 0; attempt < 2 && len(res.Sets) < MinSets; attempt++ {
			proposal, err := gen.Propose(ctx, prompt)
			if err != nil {
				slog.WarnContext(ctx, "assistant: llm failed, using fallback", "err", err, "attempt", attempt+1)
				break
			}
			var errs []string
			for _, ps := range proposal.Sets {
				set, err := p.validate(ps, GeneratorLLM)
				if err != nil {
					errs = append(errs, err.Error())
					continue
				}
				if !res.accept(set) {
					errs = append(errs, fmt.Sprintf("набор с букетом %d повторяет другой набор", ps.BouquetID))
				}
			}
			res.Rejected = append(res.Rejected, errs...)
			// один повторный запрос с перечнем ошибок (§9.1, п. 5)
			prompt.Errors = errs
		}
		if len(res.Sets) > 0 {
			res.Generator = GeneratorLLM
		}
	}
	// fallback добирает недостающие наборы
	for _, ps := range p.fallback() {
		if len(res.Sets) >= MaxSets || (len(res.Sets) >= MinSets && res.Generator == GeneratorLLM) {
			break
		}
		set, err := p.validate(ps, GeneratorFallback)
		if err != nil {
			continue // fallback строит наборы через ту же проверку, ошибка здесь — защита в глубину
		}
		res.accept(set)
	}
	return res, nil
}

// fallbackTitles — «характеры» наборов (§9.3).
var fallbackTitles = []string{"Классика", "Ярко", "Практично"}

// fallback — жадный подбор (§9.4): для 3 лучших по score букетов в бюджете добавлять подарки
// по убыванию relevance/price из разных категорий, пока помещаются; открытка — в приоритете.
func (p *pool) fallback() []ProposedSet {
	var out []ProposedSet
	for i, s := range p.bouquets {
		if len(out) == MaxSets {
			break
		}
		gifts := p.fillGifts(s.Bouquet, nil, p.budget-s.Bouquet.Price)
		reasons := slices.Clone(s.Reasons)
		if len(reasons) > 3 {
			reasons = reasons[:3]
		}
		ids := make([]int64, 0, len(gifts))
		for _, g := range gifts {
			ids = append(ids, g.ID)
			reasons = append(reasons, fmt.Sprintf("Дополнение: %s", g.Name))
		}
		out = append(out, ProposedSet{BouquetID: s.Bouquet.ID, GiftIDs: ids, Reasons: reasons, Title: fallbackTitles[i%len(fallbackTitles)]})
	}
	return out
}

// fillGifts добавляет к букету подарки из разных категорий в пределах left, проверяя набор правилами.
func (p *pool) fillGifts(b domain.Bouquet, kept []domain.GiftItem, left domain.Money) []domain.GiftItem {
	chosen := slices.Clone(kept)
	used := map[string]bool{}
	for _, g := range chosen {
		used[g.Category] = true
	}
	try := func(g scoredGift) {
		if used[g.gift.Category] || g.gift.Price > left {
			return
		}
		candidate := append(slices.Clone(chosen), g.gift)
		if rules.CheckGiftSet(b, candidate, p.budget, p.ctx.Profile).Blocked() {
			return
		}
		chosen = candidate
		used[g.gift.Category] = true
		left -= g.gift.Price
	}
	// открытка — приоритет, если осталось ≥ её цены
	for _, g := range p.gifts {
		if g.gift.Category == domain.GiftCard {
			try(g)
			break
		}
	}
	byValue := slices.Clone(p.gifts)
	slices.SortStableFunc(byValue, func(a, b scoredGift) int {
		return cmp.Compare(b.relevance/float64(b.gift.Price), a.relevance/float64(a.gift.Price))
	})
	for _, g := range byValue {
		try(g)
	}
	return chosen[len(kept):]
}
