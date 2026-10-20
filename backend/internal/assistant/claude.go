package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/shared/constant"
)

// Claude — генератор наборов на Anthropic API (§9.1). Ответ ограничен JSON-схемой через
// structured outputs (output_config.format): принудительный tool_choice на текущих моделях недоступен.
type Claude struct {
	client anthropic.Client
	model  string
}

// NewClaude создаёт клиент: таймаут на запрос (по умолчанию 20 с) и до 2 повторов с экспоненциальной
// задержкой — для 408/429/5xx и сетевых ошибок их выполняет SDK (§18.2).
func NewClaude(apiKey, model string, timeout time.Duration, opts ...option.RequestOption) *Claude {
	opts = append([]option.RequestOption{option.WithAPIKey(apiKey), option.WithRequestTimeout(timeout), option.WithMaxRetries(2)}, opts...)
	return &Claude{client: anthropic.NewClient(opts...), model: model}
}

const systemPrompt = `Ты — ассистент цветочного сервиса. Собери 2–3 набора «букет + дополнительные подарки» для дарителя.

Правила:
- Используй только позиции из присланного списка, указывая их id. Ничего не придумывай.
- В каждом наборе ровно один букет и 0–3 подарка из разных категорий; сумма цен набора не больше бюджета.
- Наборы должны отличаться букетом или хотя бы двумя подарками и иметь разный характер, например «Классика», «Ярко», «Практично».
- Причины (reasons) — 2–3 коротких предложения по-русски: почему набор подходит к поводу. Не обещай безопасность и не давай медицинских утверждений.
- Если в запросе есть errors — это ошибки прошлой попытки, исправь их.`

// proposalSchema — JSON-схема ответа (additionalProperties: false на каждом объекте).
var proposalSchema = map[string]any{
	"type":                 "object",
	"additionalProperties": false,
	"required":             []string{"sets"},
	"properties": map[string]any{
		"sets": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []string{"bouquet_id", "gift_ids", "reasons", "title"},
				"properties": map[string]any{
					"bouquet_id": map[string]any{"type": "integer"},
					"gift_ids":   map[string]any{"type": "array", "items": map[string]any{"type": "integer"}},
					"reasons":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"title":      map[string]any{"type": "string"},
				},
			},
		},
	},
}

// ErrRefused — модель отказалась отвечать (stop_reason=refusal).
var ErrRefused = errors.New("llm refused the request")

// Propose отправляет кандидатов и разбирает структурированный ответ. Текст промпта не логируется (§18.4).
func (c *Claude) Propose(ctx context.Context, p Prompt) (Proposal, error) {
	payload, err := json.Marshal(p)
	if err != nil {
		return Proposal{}, fmt.Errorf("encode prompt: %w", err)
	}
	resp, err := c.client.Beta.Messages.New(ctx, anthropic.BetaMessageNewParams{
		Model:     anthropic.Model(c.model),
		MaxTokens: 16000,
		System:    []anthropic.BetaTextBlockParam{{Text: systemPrompt}},
		Messages: []anthropic.BetaMessageParam{
			anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(string(payload))),
		},
		OutputConfig: anthropic.BetaOutputConfigParam{
			Effort: anthropic.BetaOutputConfigEffortMedium,
			Format: anthropic.BetaJSONOutputFormatParam{Schema: proposalSchema},
		},
		// при отказе модели запрос обслуживает резервная модель (server-side fallback)
		Fallbacks: anthropic.BetaFallbacksParamUnion{OfDefault: constant.ValueOf[constant.Default]()},
		Betas:     []anthropic.AnthropicBeta{anthropic.AnthropicBetaServerSideFallback2026_07_01},
	})
	if err != nil {
		var apiErr *anthropic.Error
		if errors.As(err, &apiErr) {
			return Proposal{}, fmt.Errorf("anthropic api status %d: %w", apiErr.StatusCode, err)
		}
		return Proposal{}, fmt.Errorf("anthropic request: %w", err)
	}
	switch resp.StopReason {
	case anthropic.BetaStopReasonRefusal:
		return Proposal{}, ErrRefused
	case anthropic.BetaStopReasonMaxTokens:
		return Proposal{}, errors.New("llm response truncated (max_tokens)")
	}
	for _, block := range resp.Content {
		if text, ok := block.AsAny().(anthropic.BetaTextBlock); ok {
			var out Proposal
			if err := json.Unmarshal([]byte(text.Text), &out); err != nil {
				return Proposal{}, fmt.Errorf("decode llm json: %w", err)
			}
			return out, nil
		}
	}
	return Proposal{}, errors.New("llm returned no text block")
}
