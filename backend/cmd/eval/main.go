// Command eval — офлайн-оценка ranker'а (§11, RM-04): split по получателям, метрики с bootstrap CI,
// запись в research.experiment_runs и docs/eval_report.md. Воспроизводим по -seed.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"

	"github.com/zbekxzz/gulder/backend/internal/catalog"
	"github.com/zbekxzz/gulder/backend/internal/eval"
	"github.com/zbekxzz/gulder/backend/internal/ranking"
	"github.com/zbekxzz/gulder/backend/internal/store"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "eval:", err)
		os.Exit(1)
	}
}

func run() error {
	seed := flag.Uint64("seed", 42, "seed split и bootstrap")
	dataset := flag.String("dataset", "", "версия экспертного набора (пусто — проверенная, иначе черновик)")
	report := flag.String("report", "", "куда записать markdown-отчёт")
	iters := flag.Int("bootstrap", 1000, "итераций bootstrap")
	rankingPath := flag.String("ranking", os.Getenv("RANKING_CONFIG"), "ranking.yaml")
	flag.Parse()
	if *seed > math.MaxInt32 {
		return fmt.Errorf("seed must fit int32")
	}
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	ctx := context.Background()
	st, err := store.Open(ctx, url)
	if err != nil {
		return err
	}
	defer st.Close()

	cfg, err := ranking.LoadConfig(*rankingPath)
	if err != nil {
		return err
	}
	if *dataset == "" {
		if *dataset, err = eval.LatestDataset(ctx, st.Pool); err != nil {
			return err
		}
	}
	cases, err := eval.LoadCases(ctx, st.Pool, *dataset)
	if err != nil {
		return err
	}
	opt := eval.DefaultOptions(*seed)
	opt.BootstrapIters = *iters
	snap, err := catalog.LoadSnapshot(ctx, st.Pool, opt.Now)
	if err != nil {
		return err
	}
	res := eval.Evaluate(ctx, cases, *dataset, snap, cfg, opt)
	baseID, calID, err := eval.SaveRuns(ctx, st.Pool, cfg, res)
	if err != nil {
		return err
	}
	if *report != "" {
		//nolint:gosec // путь отчёта задаёт оператор
		if err := os.WriteFile(*report, []byte(eval.Markdown(res, cfg.ModelVersion, baseID, calID)), 0o600); err != nil {
			return fmt.Errorf("write report: %w", err)
		}
	}
	out, _ := json.MarshalIndent(map[string]any{"experiment_runs": []int64{baseID, calID}, "result": res}, "", "  ")
	fmt.Println(string(out))
	return nil
}
