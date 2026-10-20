// Package metrics — метрики Prometheus (§18.4). В метки не попадают персональные данные:
// только маршрут-шаблон, метод, код ответа, коды нарушений и источник наборов.
package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Registry — собственный реестр (не глобальный): тесты и процессы не делят состояние.
var Registry = prometheus.NewRegistry()

var (
	// HTTPDuration — латентность и коды ответов по эндпоинтам.
	HTTPDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request latency by route template and status.",
		Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30},
	}, []string{"method", "route", "status"})

	// RulesBlocks — число BLOCK по кодам нарушений: главный индикатор здоровья данных и правил.
	RulesBlocks = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "rules_block_total",
		Help: "Rules Engine BLOCK decisions by violation code (recommendations and substitutions).",
	}, []string{"code"})

	// GiftSets — наборы ассистента по источнику (llm | fallback).
	GiftSets = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "gift_sets_total",
		Help: "Gift sets returned to givers by generator.",
	}, []string{"generator"})

	// LLMErrors — ошибки обращения к LLM (после повторов SDK).
	LLMErrors = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "llm_errors_total",
		Help: "LLM calls that failed and fell back to the deterministic generator.",
	})
)

func init() {
	Registry.MustRegister(HTTPDuration, RulesBlocks, GiftSets, LLMErrors,
		collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
}

// Handler отдаёт /metrics.
func Handler() http.Handler { return promhttp.HandlerFor(Registry, promhttp.HandlerOpts{}) }

// CountBlocks учитывает коды нарушений BLOCK.
func CountBlocks(codes []string) {
	for _, c := range codes {
		RulesBlocks.WithLabelValues(c).Inc()
	}
}
