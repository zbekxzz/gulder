package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/zbekxzz/gulder/backend/internal/store"
)

// DevPassword — пароль dev-пользователей. Только для APP_ENV=dev/test (§14).
const DevPassword = "gulder-dev-password" //nolint:gosec // публичный пароль dev-сида, в prod пользователи не создаются

// DevUser — пользователь, которого сид создаёт в dev.
type DevUser struct {
	Email, Name string
	Role        Role
	ShopCode    string
}

// DevUsers — по пользователю на каждую роль (+ второй даритель и флорист для проверок изоляции).
var DevUsers = []DevUser{
	{Email: "giver@gulder.local", Name: "Даритель", Role: RoleGiver},
	{Email: "giver2@gulder.local", Name: "Второй даритель", Role: RoleGiver},
	{Email: "florist@gulder.local", Name: "Флорист (Абая)", Role: RoleFlorist, ShopCode: "abay"},
	{Email: "florist2@gulder.local", Name: "Флорист (Достык)", Role: RoleFlorist, ShopCode: "dostyk"},
	{Email: "manager@gulder.local", Name: "Менеджер", Role: RoleManager},
}

// SeedDevUsers создаёт dev-пользователей; существующие пропускаются. Возвращает число созданных.
func SeedDevUsers(ctx context.Context, q store.Querier) (int, error) {
	n := 0
	for _, u := range DevUsers {
		var shopID *int64
		if u.ShopCode != "" {
			var id int64
			if err := q.QueryRow(ctx, `SELECT id FROM shops WHERE code = $1`, u.ShopCode).Scan(&id); err != nil {
				return n, fmt.Errorf("shop %s for %s: %w", u.ShopCode, u.Email, err)
			}
			shopID = &id
		}
		_, err := CreateUser(ctx, q, u.Email, DevPassword, u.Name, u.Role, shopID)
		if errors.Is(err, ErrEmailTaken) {
			continue
		}
		if err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}
