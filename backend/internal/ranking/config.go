// Package ranking — baseline-ranker (§8.1): взвешенный content score, confidence, разнообразие
// Top-5 и нумерологический фактор (§8.3). Работает только по допущенным Rules Engine позициям.
package ranking

import (
	_ "embed"
	"errors"
	"fmt"
	"math"
	"os"

	"gopkg.in/yaml.v3"
)

// Факторы score (§8.1).
const (
	FactorFlowerMatch = "flower_match"
	FactorColorMatch  = "color_match"
	FactorOccasionFit = "occasion_fit"
	FactorStyleFormat = "style_format"
	FactorBudgetFit   = "budget_fit"
	FactorSeasonAvail = "season_avail"
	FactorNeutrality  = "neutrality"
	FactorNumerology  = "numerology"
)

// BaseFactors — факторы без нумерологии (признаки модели, RM-07).
var BaseFactors = []string{
	FactorFlowerMatch, FactorColorMatch, FactorOccasionFit, FactorStyleFormat,
	FactorBudgetFit, FactorSeasonAvail, FactorNeutrality,
}

// MaxNumerologyWeight — потолок веса нумерологии (FR-N04).
const MaxNumerologyWeight = 0.15

// Platt — калибровка confidence: sigmoid(A·x + B).
type Platt struct {
	A float64 `yaml:"a" json:"a"`
	B float64 `yaml:"b" json:"b"`
}

// Apply калибрует значение.
func (p *Platt) Apply(x float64) float64 {
	if p == nil {
		return x
	}
	return 1 / (1 + math.Exp(-(p.A*x + p.B)))
}

// Config — веса и версия модели.
type Config struct {
	ModelVersion         string             `yaml:"model_version" json:"model_version"`
	Weights              map[string]float64 `yaml:"weights" json:"weights"`
	NeutralityPerUnknown float64            `yaml:"neutrality_per_unknown" json:"neutrality_per_unknown"`
	NumerologyWeight     float64            `yaml:"numerology_weight" json:"numerology_weight"`
	Calibration          *Platt             `yaml:"calibration" json:"calibration,omitempty"`
}

//go:embed ranking.yaml
var defaultYAML []byte

// DefaultConfig — встроенные веса.
func DefaultConfig() Config {
	cfg, err := ParseConfig(defaultYAML)
	if err != nil {
		panic(fmt.Sprintf("embedded ranking.yaml: %v", err)) // ошибка сборки, ловится тестом
	}
	return cfg
}

// LoadConfig читает ranking.yaml; пустой путь — встроенный.
func LoadConfig(path string) (Config, error) {
	if path == "" {
		return DefaultConfig(), nil
	}
	raw, err := os.ReadFile(path) //nolint:gosec // путь задаёт оператор через RANKING_CONFIG
	if err != nil {
		return Config{}, fmt.Errorf("read ranking config: %w", err)
	}
	return ParseConfig(raw)
}

// ParseConfig разбирает и проверяет конфиг.
func ParseConfig(raw []byte) (Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return cfg, fmt.Errorf("parse ranking config: %w", err)
	}
	return cfg, cfg.Validate()
}

// Validate проверяет, что заданы все факторы, сумма весов = 1 и нумерология ≤ 15%.
func (c Config) Validate() error {
	if c.ModelVersion == "" {
		return errors.New("ranking: model_version is required")
	}
	sum := 0.0
	for _, f := range BaseFactors {
		w, ok := c.Weights[f]
		if !ok || w < 0 {
			return fmt.Errorf("ranking: weight %q is missing or negative", f)
		}
		sum += w
	}
	if math.Abs(sum-1) > 1e-6 {
		return fmt.Errorf("ranking: base weights must sum to 1, got %.4f", sum)
	}
	if c.NumerologyWeight < 0 || c.NumerologyWeight > MaxNumerologyWeight {
		return fmt.Errorf("ranking: numerology_weight must be within [0, %.2f]", MaxNumerologyWeight)
	}
	if c.NeutralityPerUnknown < 0 {
		return errors.New("ranking: neutrality_per_unknown must be non-negative")
	}
	return nil
}

// EffectiveWeights — веса для конкретного запроса: нейтральность усиливается на 0.05 за каждое
// неизвестное поле безопасности с перенормировкой (FR-R02); при включённой нумерологии остальные
// веса масштабируются на (1 − w) (FR-N04).
func (c Config) EffectiveWeights(unknownSafetyFields int, numerology bool) map[string]float64 {
	w := map[string]float64{}
	sum := 0.0
	for _, f := range BaseFactors {
		w[f] = c.Weights[f]
		if f == FactorNeutrality {
			w[f] += c.NeutralityPerUnknown * float64(unknownSafetyFields)
		}
		sum += w[f]
	}
	scale := 1.0
	if numerology && c.NumerologyWeight > 0 {
		scale = 1 - c.NumerologyWeight
		w[FactorNumerology] = c.NumerologyWeight
	}
	for _, f := range BaseFactors {
		w[f] = w[f] / sum * scale
	}
	return w
}
