// Package rules — Rules Engine, ядро безопасности Gulder (§7 TASK.md).
//
// Чистый Go без зависимостей от БД, HTTP и LLM. Вызывается из рекомендаций, ассистента,
// фильтров, валидации замен и офлайн-оценки — до ranker'а. Принципы (§7.1):
//  1. «Не знаю» ≠ «нет ограничения»: неизвестное поле не даёт ALLOW рискованной позиции —
//     WARN, штраф к confidence и запись в missing_info.
//  2. «Предполагаю» трактуется как «знаю».
//  3. Неопознанное растение, неизвестная токсичность для указанного питомца, неизвестный
//     состав подарка при известных аллергиях → BLOCK.
//  4. BLOCK окончательный: никакой другой слой не может его снять.
package rules

import (
	"slices"

	"github.com/zbekxzz/gulder/backend/internal/domain"
)

// Version — версия правил; пишется в recommendation_runs.rules_version.
const Version = "rules-1.0.0"

// Коды нарушений (§7.3–7.6).
const (
	CodeUnverifiedPlant      = "UNVERIFIED_PLANT"
	CodeAllergenMatch        = "ALLERGEN_MATCH"
	CodeAllergyUnknown       = "ALLERGY_UNKNOWN"
	CodePetNoData            = "PET_NO_DATA"
	CodePetToxic             = "PET_TOXIC"
	CodePetsUnknown          = "PETS_UNKNOWN"
	CodeDislikedFlower       = "DISLIKED_FLOWER"
	CodeDislikedColor        = "DISLIKED_COLOR"
	CodeEmptyComposition     = "EMPTY_COMPOSITION"
	CodeAsthmaTrigger        = "ASTHMA_TRIGGER"
	CodeAsthmaCaution        = "ASTHMA_CAUTION"
	CodeAsthmaUnknown        = "ASTHMA_UNKNOWN"
	CodeScentTooStrong       = "SCENT_TOO_STRONG"
	CodeScentCaution         = "SCENT_CAUTION"
	CodeScentUnknown         = "SCENT_UNKNOWN"
	CodeUnknownComposition   = "UNKNOWN_COMPOSITION"
	CodePetHazard            = "PET_HAZARD"
	CodeOverBudget           = "OVER_BUDGET"
	CodeDuplicateItem        = "DUPLICATE_ITEM"
	CodeRecipientRestriction = "RECIPIENT_RESTRICTION"
)

// Виды проверяемых объектов (Result.Kind).
const (
	KindBouquet = "bouquet"
	KindGift    = "gift"
	KindGiftSet = "gift_set"
)

// Violation — одно сработавшее правило.
type Violation struct {
	Code              string          `json:"code"`
	Decision          domain.Decision `json:"decision"`
	Message           string          `json:"message"`
	Attribute         string          `json:"attribute,omitempty"`
	Source            domain.Source   `json:"source,omitempty"`
	Item              string          `json:"item,omitempty"`
	ConfidencePenalty float64         `json:"confidence_penalty,omitempty"`
}

// Result — итог проверки позиции.
type Result struct {
	Kind        string      `json:"kind"`
	ItemID      int64       `json:"item_id"`
	Violations  []Violation `json:"violations"`
	MissingInfo []string    `json:"missing_info"` // множество, по возрастанию
}

func newResult(kind string, id int64) Result {
	return Result{Kind: kind, ItemID: id, Violations: []Violation{}, MissingInfo: []string{}}
}

// add добавляет нарушение; дубликаты (code, item, attribute) схлопываются (§7.5).
func (r *Result) add(v Violation) {
	for _, existing := range r.Violations {
		if existing.Code == v.Code && existing.Item == v.Item && existing.Attribute == v.Attribute {
			return
		}
	}
	r.Violations = append(r.Violations, v)
}

func (r *Result) missing(attr string) {
	if !slices.Contains(r.MissingInfo, attr) {
		r.MissingInfo = append(r.MissingInfo, attr)
		slices.Sort(r.MissingInfo)
	}
}

func (r *Result) merge(other Result) {
	for _, v := range other.Violations {
		r.add(v)
	}
	for _, m := range other.MissingInfo {
		r.missing(m)
	}
}

// Decision возвращает BLOCK, если есть хоть один BLOCK; иначе WARN, если есть WARN; иначе ALLOW.
func (r Result) Decision() domain.Decision {
	d := domain.Allow
	for _, v := range r.Violations {
		switch v.Decision {
		case domain.Block:
			return domain.Block
		case domain.Warn:
			d = domain.Warn
		}
	}
	return d
}

// Blocked сообщает, что позиция запрещена.
func (r Result) Blocked() bool { return r.Decision() == domain.Block }

// ConfidenceMultiplier = max(0.2, 1 − Σ штрафов WARN).
func (r Result) ConfidenceMultiplier() float64 {
	m := 1.0
	for _, v := range r.Violations {
		if v.Decision == domain.Warn {
			m -= v.ConfidencePenalty
		}
	}
	return max(0.2, m)
}

// Codes возвращает коды нарушений (для метрик и логов, без персональных данных).
func (r Result) Codes() []string {
	out := make([]string, 0, len(r.Violations))
	for _, v := range r.Violations {
		out = append(out, v.Code)
	}
	return out
}

// GiverReason — причина, которую видит даритель.
type GiverReason struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// GiverView — результат для дарителя (раздел 10 ТЗ, §7.6).
type GiverView struct {
	Suitable    bool            `json:"suitable"`
	Decision    domain.Decision `json:"decision"`
	Reasons     []GiverReason   `json:"reasons"`
	MissingInfo []string        `json:"missing_info"`
}

const recipientRestrictionMessage = "Не подходит по ограничениям получателя"

// GiverView скрывает причины, вызванные ответами получателя: дарителю показывается только
// RECIPIENT_RESTRICTION без названия аллергена, растения-причины и т.п. Повторы схлопываются.
// Все ответы API для роли giver проходят через этот метод.
func (r Result) GiverView() GiverView {
	gv := GiverView{
		Suitable:    !r.Blocked(),
		Decision:    r.Decision(),
		Reasons:     []GiverReason{},
		MissingInfo: append([]string{}, r.MissingInfo...),
	}
	hiddenAdded := false
	for _, v := range r.Violations {
		reason := GiverReason{Code: v.Code, Message: v.Message}
		if v.Source == domain.SourceRecipient {
			if hiddenAdded {
				continue
			}
			hiddenAdded = true
			reason = GiverReason{Code: CodeRecipientRestriction, Message: recipientRestrictionMessage}
		}
		if !slices.Contains(gv.Reasons, reason) {
			gv.Reasons = append(gv.Reasons, reason)
		}
	}
	return gv
}
