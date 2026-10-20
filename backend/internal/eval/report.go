package eval

import (
	"fmt"
	"strings"
)

// Targets — пороги §11 для отметки в отчёте.
const (
	TargetCoverage = 0.30
)

func check(ok bool) string {
	if ok {
		return "✅"
	}
	return "❌"
}

// Markdown — отчёт docs/eval_report.md.
func Markdown(res Result, modelVersion string, baselineID, calibratedID int64) string {
	b, c := res.Baseline, res.Calibrated
	var sb strings.Builder
	fmt.Fprintf(&sb, "# Отчёт офлайн-оценки\n\n")
	fmt.Fprintf(&sb, "Сгенерирован `make eval` (`cmd/eval`). Не править руками — перезапустите оценку.\n\n")
	fmt.Fprintf(&sb, "| Параметр | Значение |\n|---|---|\n")
	fmt.Fprintf(&sb, "| Набор | `%s` |\n| Split | `%s` (по получателям) |\n| Seed | %d |\n", res.Dataset, res.SplitID, res.Options.Seed)
	fmt.Fprintf(&sb, "| Train / test случаев | %d / %d (получателей в test: %d) |\n", b.TrainCases, b.TestCases, b.TestRecipients)
	fmt.Fprintf(&sb, "| Bootstrap | %d итераций по получателям |\n| Нумерология | выключена (FR-N06) |\n", res.Options.BootstrapIters)
	fmt.Fprintf(&sb, "| experiment_runs | baseline #%d, calibrated #%d |\n\n", baselineID, calibratedID)
	if strings.HasPrefix(res.Dataset, "draft-") {
		sb.WriteString("> ⚠️ Набор — **черновик**: оценки выставлены эвристикой `cmd/gendraft`, а не экспертом. Цифры качества\n" +
			"> ранжирования здесь проверяют пайплайн, а не модель. Метрики для model card считаются по проверенной версии (M7).\n\n")
	}
	fmt.Fprintf(&sb, "## Метрики (test)\n\n| Метрика | %s | %s+platt | Требование |\n|---|---|---|---|\n", modelVersion, modelVersion)
	fmt.Fprintf(&sb, "| NDCG@5 (95%% CI) | %.3f [%.3f; %.3f] | %.3f [%.3f; %.3f] | выше baseline (P1-модель) |\n",
		b.NDCG5.Mean, b.NDCG5.CI95[0], b.NDCG5.CI95[1], c.NDCG5.Mean, c.NDCG5.CI95[0], c.NDCG5.CI95[1])
	fmt.Fprintf(&sb, "| Unsafe rate (размеченные) | %.4f | %.4f | 0 %s |\n", b.UnsafeRate, c.UnsafeRate, check(b.UnsafeRate == 0))
	fmt.Fprintf(&sb, "| Unsafe (Rules Engine по выдаче каталога) | %.4f | %.4f | 0 %s |\n", b.UnsafeCatalogRate, c.UnsafeCatalogRate, check(b.UnsafeCatalogRate == 0))
	fmt.Fprintf(&sb, "| Нарушения бюджета в наборах | %d | %d | 0 %s |\n", b.BudgetViolations, c.BudgetViolations, check(b.BudgetViolations == 0))
	fmt.Fprintf(&sb, "| Coverage каталога в Top-5 | %.3f | %.3f | ≥ %.2f %s |\n", b.Coverage, c.Coverage, TargetCoverage, check(b.Coverage >= TargetCoverage))
	fmt.Fprintf(&sb, "| ECE (10 бинов) | %.4f | %.4f | ниже baseline %s |\n", b.ECE, c.ECE, check(c.ECE < b.ECE))
	fmt.Fprintf(&sb, "| Robustness: падение NDCG@5 при удалении %.0f%% полей (%d повторов) | %.3f | %.3f | измерить |\n\n",
		b.Robustness.DropFrac*100, b.Robustness.Repeats, b.Robustness.NDCGDrop, c.Robustness.NDCGDrop)
	fmt.Fprintf(&sb, "Калибровка: sigmoid(%.3f·x %+.3f), подобрана на train.\n", res.Platt.A, res.Platt.B)
	return sb.String()
}
