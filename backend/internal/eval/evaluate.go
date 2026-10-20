package eval

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/zbekxzz/gulder/backend/internal/assistant"
	"github.com/zbekxzz/gulder/backend/internal/catalog"
	"github.com/zbekxzz/gulder/backend/internal/domain"
	"github.com/zbekxzz/gulder/backend/internal/ranking"
	"github.com/zbekxzz/gulder/backend/internal/recipients"
	"github.com/zbekxzz/gulder/backend/internal/recommend"
	"github.com/zbekxzz/gulder/backend/internal/rules"
	"github.com/zbekxzz/gulder/backend/internal/store"
)

// Judgement — экспертная оценка пары «случай + букет».
type Judgement struct {
	BouquetID int64
	Grade     int
	Unsafe    bool
}

// Case — получатель + повод + размеченные букеты.
type Case struct {
	ID           string
	RecipientKey string // для split по получателю
	Profile      domain.Profile
	Occasion     recipients.Occasion
	Judgements   []Judgement
}

type snapshotAttr struct {
	Value      any    `json:"value"`
	Source     string `json:"source"`
	Confidence string `json:"confidence"`
}

type occasionSnapshot struct {
	Type        string       `json:"type"`
	EmotionGoal string       `json:"emotion_goal"`
	BudgetMin   domain.Money `json:"budget_min"`
	BudgetMax   domain.Money `json:"budget_max"`
	Date        *string      `json:"date"`
}

// RecipientKey — ключ получателя из case_id вида "r07-c2".
func RecipientKey(caseID string) string {
	k, _, _ := strings.Cut(caseID, "-c")
	return k
}

// LatestDataset — проверенная версия набора, если есть, иначе черновик (§12).
func LatestDataset(ctx context.Context, q store.Querier) (string, error) {
	var v string
	err := q.QueryRow(ctx, `SELECT dataset_version FROM expert_judgements
		GROUP BY dataset_version ORDER BY (dataset_version LIKE 'draft-%') ASC, max(created_at) DESC LIMIT 1`).Scan(&v)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", errors.New("нет экспертной разметки: выполните make seed")
	}
	if err != nil {
		return "", fmt.Errorf("latest dataset: %w", err)
	}
	return v, nil
}

// LoadCases загружает случаи набора.
func LoadCases(ctx context.Context, q store.Querier, version string) ([]Case, error) {
	rows, err := q.Query(ctx, `SELECT case_id, recipient_snapshot, occasion_snapshot, bouquet_id, grade, is_unsafe
		FROM expert_judgements WHERE dataset_version = $1 ORDER BY case_id, bouquet_id`, version)
	if err != nil {
		return nil, fmt.Errorf("load judgements: %w", err)
	}
	defer rows.Close()
	var out []Case
	for rows.Next() {
		var caseID string
		var rawProfile, rawOccasion []byte
		var j Judgement
		if err := rows.Scan(&caseID, &rawProfile, &rawOccasion, &j.BouquetID, &j.Grade, &j.Unsafe); err != nil {
			return nil, fmt.Errorf("scan judgement: %w", err)
		}
		if len(out) == 0 || out[len(out)-1].ID != caseID {
			c, err := newCase(caseID, rawProfile, rawOccasion)
			if err != nil {
				return nil, err
			}
			out = append(out, c)
		}
		out[len(out)-1].Judgements = append(out[len(out)-1].Judgements, j)
	}
	return out, rows.Err()
}

func newCase(id string, rawProfile, rawOccasion []byte) (Case, error) {
	var attrs map[string]snapshotAttr
	if err := json.Unmarshal(rawProfile, &attrs); err != nil {
		return Case{}, fmt.Errorf("case %s profile: %w", id, err)
	}
	var occ occasionSnapshot
	if err := json.Unmarshal(rawOccasion, &occ); err != nil {
		return Case{}, fmt.Errorf("case %s occasion: %w", id, err)
	}
	p := domain.Profile{}
	for k, a := range attrs {
		p[k] = domain.Attribute{Value: a.Value, Source: domain.Source(a.Source), Confidence: domain.Confidence(a.Confidence)}
	}
	return Case{
		ID: id, RecipientKey: RecipientKey(id), Profile: p,
		Occasion: recipients.Occasion{Type: occ.Type, EmotionGoal: occ.EmotionGoal, BudgetMin: occ.BudgetMin, BudgetMax: occ.BudgetMax, Date: occ.Date},
	}, nil
}

// Options — параметры запуска оценки (пишутся в experiment_runs.params, RM-04).
type Options struct {
	Seed              uint64    `json:"seed"`
	TestFrac          float64   `json:"test_frac"`
	BootstrapIters    int       `json:"bootstrap_iters"`
	RobustnessRepeats int       `json:"robustness_repeats"`
	DropFrac          float64   `json:"drop_frac"`
	Now               time.Time `json:"now"`
}

// DefaultOptions — значения §11.
func DefaultOptions(seed uint64) Options {
	return Options{
		Seed: seed, TestFrac: 0.3, BootstrapIters: 1000, RobustnessRepeats: 5, DropFrac: 0.3,
		Now: time.Date(2026, 11, 1, 12, 0, 0, 0, time.FixedZone("ALMT", 5*3600)),
	}
}

// Robustness — падение NDCG@5 при случайном удалении доли полей профиля.
type Robustness struct {
	NDCGDrop float64 `json:"ndcg_drop"`
	NDCGMean float64 `json:"ndcg_mean"`
	Repeats  int     `json:"repeats"`
	DropFrac float64 `json:"drop_frac"`
}

// Metrics — метрики одного запуска (§11).
type Metrics struct {
	NDCG5             CI         `json:"ndcg@5"`
	UnsafeRate        float64    `json:"unsafe_rate"`         // размеченные is_unsafe среди выданного
	UnsafeCatalogRate float64    `json:"unsafe_catalog_rate"` // Rules Engine на эталонном профиле по выдаче всего каталога
	BudgetViolations  int        `json:"budget_violations"`   // наборы ассистента с суммой > бюджета
	Coverage          float64    `json:"coverage"`
	ECE               float64    `json:"ece"`
	Robustness        Robustness `json:"robustness"`
	TestCases         int        `json:"test_cases"`
	TestRecipients    int        `json:"test_recipients"`
	TrainCases        int        `json:"train_cases"`
}

// Result — baseline и калиброванная модель на одном split (RM-05).
type Result struct {
	SplitID    string        `json:"split_id"`
	Dataset    string        `json:"dataset_version"`
	Baseline   Metrics       `json:"baseline"`
	Calibrated Metrics       `json:"calibrated"`
	Platt      ranking.Platt `json:"platt"`
	Options    Options       `json:"options"`
}

// caseRun — то, что модель выдала для случая.
type caseRun struct {
	ndcg       float64
	ranked     []ranking.Scored
	judged     map[int64]Judgement
	catalogTop []ranking.Scored
	rc         *recommend.Context
}

func runCase(c Case, snap catalog.Snapshot, cfg ranking.Config, now time.Time) caseRun {
	rc := &recommend.Context{Occasion: c.Occasion, Profile: c.Profile, Snapshot: snap, Now: now}
	rc.Allowed, rc.Excluded = rules.FilterCandidates(snap.Bouquets, c.Profile)
	// FR-N06: нумерология в оценке выключена всегда
	rc.Scored = cfg.Score(ranking.Input{Candidates: rc.Allowed, Profile: c.Profile, Occasion: recommend.RankingOccasion(c.Occasion, now), Now: now})

	judged := map[int64]Judgement{}
	all := make([]int, 0, len(c.Judgements))
	for _, j := range c.Judgements {
		judged[j.BouquetID] = j
		all = append(all, j.Grade)
	}
	var ranked []ranking.Scored
	var grades []int
	for _, s := range rc.Scored {
		if j, ok := judged[s.Bouquet.ID]; ok && !s.OverBudget {
			ranked = append(ranked, s)
			grades = append(grades, j.Grade)
		}
		if len(ranked) == 5 {
			break
		}
	}
	return caseRun{ndcg: NDCGAt(5, grades, all), ranked: ranked, judged: judged, catalogTop: ranking.SelectTop(rc.Scored, recommend.TopN), rc: rc}
}

// Evaluate считает метрики на test-части; калибровка Platt подбирается только на train (blind test).
func Evaluate(ctx context.Context, cases []Case, dataset string, snap catalog.Snapshot, cfg ranking.Config, opt Options) Result {
	train, test := GroupSplit(cases, opt.Seed, opt.TestFrac)
	res := Result{
		SplitID: fmt.Sprintf("group/seed=%d/test=%.2f/%s", opt.Seed, opt.TestFrac, dataset),
		Dataset: dataset, Options: opt,
	}
	cfg.Calibration = nil // baseline — без калибровки

	// калибровка на train
	var trX []float64
	var trY []bool
	for _, c := range train {
		r := runCase(c, snap, cfg, opt.Now)
		for _, s := range r.ranked {
			trX = append(trX, s.Confidence)
			trY = append(trY, r.judged[s.Bouquet.ID].Grade >= 2)
		}
	}
	res.Platt = FitPlatt(trX, trY)

	base := Metrics{TestCases: len(test), TrainCases: len(train)}
	byRecipient := map[string][]float64{}
	var conf []float64
	var relevant []bool
	shown, unsafeShown, catalogShown, catalogUnsafe := 0, 0, 0, 0
	covered := map[int64]bool{}
	for _, c := range test {
		r := runCase(c, snap, cfg, opt.Now)
		byRecipient[c.RecipientKey] = append(byRecipient[c.RecipientKey], r.ndcg)
		for _, s := range r.ranked {
			shown++
			if r.judged[s.Bouquet.ID].Unsafe {
				unsafeShown++
			}
			conf = append(conf, s.Confidence)
			relevant = append(relevant, r.judged[s.Bouquet.ID].Grade >= 2)
		}
		// автоматическая проверка: Rules Engine на эталонном профиле для всех выданных позиций (§11)
		for _, s := range r.catalogTop {
			catalogShown++
			covered[s.Bouquet.ID] = true
			if rules.CheckBouquet(s.Bouquet, c.Profile).Blocked() {
				catalogUnsafe++
			}
		}
		// нарушения бюджета в наборах ассистента (fallback, без сети)
		if sets, err := assistant.Assemble(ctx, nil, r.rc, c.Occasion.BudgetMax); err == nil {
			for _, s := range sets.Sets {
				if s.Total > c.Occasion.BudgetMax {
					base.BudgetViolations++
				}
			}
		}
	}
	perRecipient := make([]float64, 0, len(byRecipient))
	keys := make([]string, 0, len(byRecipient))
	for k := range byRecipient {
		keys = append(keys, k)
	}
	slices.Sort(keys) // порядок групп фиксирован — bootstrap воспроизводим
	for _, k := range keys {
		perRecipient = append(perRecipient, mean(byRecipient[k]))
	}
	base.TestRecipients = len(keys)
	base.NDCG5 = Bootstrap(perRecipient, opt.BootstrapIters, opt.Seed)
	base.UnsafeRate = ratio(unsafeShown, shown)
	base.UnsafeCatalogRate = ratio(catalogUnsafe, catalogShown)
	base.Coverage = ratio(len(covered), len(snap.Bouquets))
	base.ECE = ECE(conf, relevant, 10)
	base.Robustness = robustness(test, snap, cfg, opt, base.NDCG5.Mean)

	cal := base
	calibrated := make([]float64, len(conf))
	for i, x := range conf {
		calibrated[i] = res.Platt.Apply(x)
	}
	cal.ECE = ECE(calibrated, relevant, 10)
	res.Baseline, res.Calibrated = base, cal
	return res
}

// robustness: поле профиля → unknown с вероятностью DropFrac, RobustnessRepeats повторов (§11).
func robustness(test []Case, snap catalog.Snapshot, cfg ranking.Config, opt Options, baseMean float64) Robustness {
	rng := rand.New(rand.NewPCG(opt.Seed, 0x7055)) //nolint:gosec // воспроизводимость по seed (§11)
	var means []float64
	for r := 0; r < opt.RobustnessRepeats; r++ {
		var vals []float64
		for _, c := range test {
			dropped := c
			dropped.Profile = domain.Profile{}
			keys := make([]string, 0, len(c.Profile))
			for k := range c.Profile {
				keys = append(keys, k)
			}
			slices.Sort(keys)
			for _, k := range keys {
				if rng.Float64() >= opt.DropFrac {
					dropped.Profile[k] = c.Profile[k]
				}
			}
			vals = append(vals, runCase(dropped, snap, cfg, opt.Now).ndcg)
		}
		means = append(means, mean(vals))
	}
	m := mean(means)
	return Robustness{NDCGDrop: baseMean - m, NDCGMean: m, Repeats: opt.RobustnessRepeats, DropFrac: opt.DropFrac}
}

func mean(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := 0.0
	for _, x := range xs {
		s += x
	}
	return s / float64(len(xs))
}

func ratio(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b)
}

// SaveRuns пишет baseline и калиброванную модель в research.experiment_runs (RM-04).
func SaveRuns(ctx context.Context, q store.Querier, cfg ranking.Config, res Result) (baselineID, calibratedID int64, err error) {
	params := map[string]any{"options": res.Options, "weights": cfg.Weights, "neutrality_per_unknown": cfg.NeutralityPerUnknown}
	insert := func(model string, m Metrics, extra map[string]any) (int64, error) {
		p := map[string]any{}
		for k, v := range params {
			p[k] = v
		}
		for k, v := range extra {
			p[k] = v
		}
		var id int64
		err := q.QueryRow(ctx, `INSERT INTO research.experiment_runs (model_version, dataset_version, split_id, seed, params, metrics, numerology_in_features)
			VALUES ($1,$2,$3,$4,$5,$6,false) RETURNING id`, model, res.Dataset, res.SplitID, int64(res.Options.Seed), p, m).Scan(&id)
		if err != nil {
			return 0, fmt.Errorf("save experiment run: %w", err)
		}
		return id, nil
	}
	if baselineID, err = insert(cfg.ModelVersion, res.Baseline, nil); err != nil {
		return 0, 0, err
	}
	calibratedID, err = insert(cfg.ModelVersion+"+platt", res.Calibrated, map[string]any{"calibration": res.Platt})
	return baselineID, calibratedID, err
}
