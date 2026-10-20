// Command seed загружает data/*.csv в БД (§12). Идемпотентен: повторный запуск
// не создаёт дублей и не затирает ручные правки каталога (§18.3).
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5"

	"github.com/zbekxzz/gulder/backend/internal/auth"
	"github.com/zbekxzz/gulder/backend/internal/catalog"
	"github.com/zbekxzz/gulder/backend/internal/config"
	"github.com/zbekxzz/gulder/backend/internal/store"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "seed:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	dataDir := flag.String("data", cfg.DataDir, "каталог с CSV")
	flag.Parse()

	ctx := context.Background()
	st, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer st.Close()

	var rep catalog.SeedReport
	users := 0
	err = st.InTx(ctx, func(tx pgx.Tx) error {
		var err error
		if rep, err = catalog.Seed(ctx, tx, *dataDir); err != nil {
			return err
		}
		// §14: пользователи на каждую роль — только в dev
		if cfg.IsDev() {
			users, err = auth.SeedDevUsers(ctx, tx)
		}
		return err
	})
	if err != nil {
		return err
	}
	slog.Info("seed done", "taxa", rep.Taxa, "shops", rep.Shops, "flowers", rep.Flowers,
		"bouquets", rep.Bouquets, "gifts", rep.Gifts, "numerology_rows", rep.NumerologyRows, "judgements", rep.Judgements, "dev_users", users)
	return nil
}
