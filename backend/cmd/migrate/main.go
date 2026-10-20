// Command migrate применяет миграции goose: `migrate up|down|reset` (§18.3, §18.6).
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/zbekxzz/gulder/backend/internal/store"
)

func main() {
	dir := "up"
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		fmt.Fprintln(os.Stderr, "DATABASE_URL is required")
		os.Exit(2)
	}
	if err := store.Migrate(context.Background(), url, dir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("migrate", dir, "ok")
}
