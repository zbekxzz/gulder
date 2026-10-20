package httpapi_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthFlow(t *testing.T) {
	e := newEnv(t)

	r := e.do(http.MethodPost, "/api/v1/auth/register", "", map[string]string{"email": "New@Example.com", "password": "long-enough-pass"})
	require.Equal(t, http.StatusCreated, r.Status, string(r.Body))
	body := r.JSON(t)
	assert.Equal(t, "giver", body["user"].(map[string]any)["role"], "регистрация создаёт только дарителя")

	r = e.do(http.MethodPost, "/api/v1/auth/register", "", map[string]string{"email": "new@example.com", "password": "long-enough-pass"})
	assert.Equal(t, http.StatusConflict, r.Status)

	r = e.do(http.MethodPost, "/api/v1/auth/register", "", map[string]string{"email": "bad", "password": "short"})
	assert.Equal(t, http.StatusUnprocessableEntity, r.Status)

	r = e.do(http.MethodPost, "/api/v1/auth/login", "", map[string]string{"email": "new@example.com", "password": "wrong-password"})
	assert.Equal(t, http.StatusUnauthorized, r.Status)

	r = e.do(http.MethodPost, "/api/v1/auth/refresh", "", map[string]string{"refresh_token": body["refresh_token"].(string)})
	require.Equal(t, http.StatusOK, r.Status)
	// access-токен нельзя использовать как refresh
	r = e.do(http.MethodPost, "/api/v1/auth/refresh", "", map[string]string{"refresh_token": body["access_token"].(string)})
	assert.Equal(t, http.StatusUnauthorized, r.Status)

	r = e.do(http.MethodGet, "/api/v1/me", body["access_token"].(string), nil)
	require.Equal(t, http.StatusOK, r.Status)
	assert.Equal(t, "new@example.com", r.JSON(t)["email"])
}

// §14: без Authorization → 401; не та роль → 403; чужой ресурс → 404.
func TestAccessControl(t *testing.T) {
	e := newEnv(t)
	giver, giver2, florist := e.login("giver@gulder.local"), e.login("giver2@gulder.local"), e.login("florist@gulder.local")

	assert.Equal(t, http.StatusUnauthorized, e.do(http.MethodGet, "/api/v1/recipients", "", nil).Status)
	assert.Equal(t, http.StatusUnauthorized, e.do(http.MethodGet, "/api/v1/recipients", "garbage", nil).Status)
	assert.Equal(t, http.StatusForbidden, e.do(http.MethodGet, "/api/v1/recipients", florist, nil).Status)
	assert.Equal(t, http.StatusForbidden, e.do(http.MethodGet, "/api/v1/catalog/bouquets", giver, nil).Status)

	id := e.createRecipient(giver, "Мама")
	path := fmt.Sprintf("/api/v1/recipients/%d", id)
	assert.Equal(t, http.StatusNotFound, e.do(http.MethodGet, path, giver2, nil).Status, "чужой ID → 404")
	assert.Equal(t, http.StatusNotFound, e.do(http.MethodPut, path, giver2, map[string]any{"attributes": []any{}}).Status)
	assert.Equal(t, http.StatusNotFound, e.do(http.MethodDelete, path, giver2, nil).Status)
	r := e.do(http.MethodPost, "/api/v1/occasions", giver2, map[string]any{"recipient_id": id, "type": "birthday", "budget_max": 1000})
	assert.Equal(t, http.StatusNotFound, r.Status)
	assert.Equal(t, http.StatusOK, e.do(http.MethodGet, path, giver, nil).Status)

	list := e.do(http.MethodGet, "/api/v1/recipients", giver2, nil).JSON(t)
	assert.Empty(t, list["items"])
}

// FR-P01–P03: CRUD получателя, у каждого поля value/source/confidence/updated_at.
func TestRecipientCRUD_FRP01_FRP03(t *testing.T) {
	e := newEnv(t)
	giver := e.login("giver@gulder.local")
	id := e.createRecipient(giver, "Айгерим",
		attr("allergies", []string{"genus:lilium", "FOOD:nuts"}, "know"),
		attr("pets", []string{"cat"}, "assume"),
		attr("asthma", nil, "unknown"),
	)
	path := fmt.Sprintf("/api/v1/recipients/%d", id)
	got := e.do(http.MethodGet, path, giver, nil).JSON(t)
	attrs := byKey(got["attributes"].([]any))
	assert.Equal(t, []any{"genus:lilium", "food:nuts"}, attrs["allergies"]["value"])
	assert.Equal(t, "giver", attrs["allergies"]["source"])
	assert.NotEmpty(t, attrs["allergies"]["updated_at"])
	assert.Equal(t, "assume", attrs["pets"]["confidence"])
	// все ключи профиля присутствуют; незаполненные — unknown без значения
	assert.Len(t, attrs, 11)
	assert.Equal(t, "unknown", attrs["style"]["confidence"])
	assert.NotContains(t, attrs["style"], "value")

	r := e.do(http.MethodPut, path, giver, map[string]any{"nickname": "Айгерим К.", "attributes": []any{attr("style", "minimal", "know")}})
	require.Equal(t, http.StatusOK, r.Status, string(r.Body))
	upd := r.JSON(t)
	assert.Equal(t, "Айгерим К.", upd["nickname"])
	assert.Equal(t, "minimal", byKey(upd["attributes"].([]any))["style"]["value"])
	assert.Equal(t, []any{"genus:lilium", "food:nuts"}, byKey(upd["attributes"].([]any))["allergies"]["value"], "PUT не стирает другие поля")

	r = e.do(http.MethodPut, path, giver, map[string]any{"attributes": []any{
		attr("allergies", []string{"lilies"}, "know"), attr("scent_sensitivity", "huge", "know"), attr("unknown_key", 1, "know"),
		map[string]any{"key": "pets", "value": []string{}, "source": "recipient", "confidence": "know"},
	}})
	require.Equal(t, http.StatusUnprocessableEntity, r.Status)
	details := r.JSON(t)["error"].(map[string]any)["details"].(map[string]any)
	assert.Contains(t, details, "allergies")
	assert.Contains(t, details, "scent_sensitivity")
	assert.Contains(t, details, "unknown_key")
	assert.Contains(t, details, "pets", "даритель не может выдавать себя за получателя")
}

// AC-2 (DTO/HTTP): «не знаю» не превращается в «аллергий нет».
func TestAC2_UnknownIsNotNoAllergy_HTTP(t *testing.T) {
	e := newEnv(t)
	giver := e.login("giver@gulder.local")
	id := e.createRecipient(giver, "Коллега", attr("allergies", nil, "unknown"))
	path := fmt.Sprintf("/api/v1/recipients/%d", id)
	a := byKey(e.do(http.MethodGet, path, giver, nil).JSON(t)["attributes"].([]any))["allergies"]
	assert.Equal(t, "unknown", a["confidence"])
	assert.NotContains(t, a, "value", "нет значения, а не пустой список")

	// «знаю» без значения — ошибка, а не «аллергий нет»
	r := e.do(http.MethodPut, path, giver, map[string]any{"attributes": []any{attr("allergies", nil, "know")}})
	assert.Equal(t, http.StatusUnprocessableEntity, r.Status)
	// «не знаю» со значением — тоже ошибка: при «не знаю» значение не вводится
	r = e.do(http.MethodPut, path, giver, map[string]any{"attributes": []any{attr("allergies", []string{}, "unknown")}})
	assert.Equal(t, http.StatusUnprocessableEntity, r.Status)
	// явное «аллергий нет» — пустой список с know
	r = e.do(http.MethodPut, path, giver, map[string]any{"attributes": []any{attr("allergies", []string{}, "know")}})
	require.Equal(t, http.StatusOK, r.Status)
	a = byKey(r.JSON(t)["attributes"].([]any))["allergies"]
	assert.Equal(t, []any{}, a["value"])
	assert.Equal(t, "know", a["confidence"])
}

// §13: ответы получателя скрыты от дарителя: {"key","hidden":true} без значения; изменить их даритель не может.
func TestRecipientAnswersHiddenFromGiver(t *testing.T) {
	e := newEnv(t)
	giver := e.login("giver@gulder.local")
	id := e.createRecipient(giver, "Сестра")
	_, err := e.store.Pool.Exec(context.Background(), `INSERT INTO recipient_attributes (recipient_id, key, value, source, confidence)
		VALUES ($1, 'allergies', '["genus:lilium"]', 'recipient', 'know')`, id)
	require.NoError(t, err)

	path := fmt.Sprintf("/api/v1/recipients/%d", id)
	r := e.do(http.MethodGet, path, giver, nil)
	assert.NotContains(t, string(r.Body), "lilium")
	a := byKey(r.JSON(t)["attributes"].([]any))["allergies"]
	assert.Equal(t, map[string]any{"key": "allergies", "hidden": true}, a)

	r = e.do(http.MethodPut, path, giver, map[string]any{"attributes": []any{attr("allergies", []string{}, "know")}})
	assert.Equal(t, http.StatusConflict, r.Status)
}

// FR-N07, §13: удаление получателя каскадно удаляет атрибуты, включая дату рождения; пишется audit.
func TestDeleteRecipientCascades_FRN07(t *testing.T) {
	e := newEnv(t)
	giver := e.login("giver@gulder.local")
	id := e.createRecipient(giver, "Друг", attr("birth_date", "1990-03-15", "know"), attr("pets", []string{"dog"}, "know"))
	e.createOccasion(giver, id, 20000, nil)

	r := e.do(http.MethodDelete, fmt.Sprintf("/api/v1/recipients/%d", id), giver, nil)
	require.Equal(t, http.StatusNoContent, r.Status)

	ctx := context.Background()
	var attrs, occasions, auditN int
	require.NoError(t, e.store.Pool.QueryRow(ctx, `SELECT count(*) FROM recipient_attributes WHERE recipient_id=$1`, id).Scan(&attrs))
	require.NoError(t, e.store.Pool.QueryRow(ctx, `SELECT count(*) FROM occasions WHERE recipient_id=$1`, id).Scan(&occasions))
	require.NoError(t, e.store.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE action='recipient.deleted' AND entity_id=$1`, id).Scan(&auditN))
	assert.Zero(t, attrs)
	assert.Zero(t, occasions)
	assert.Equal(t, 1, auditN)

	var birth string
	require.NoError(t, e.store.Pool.QueryRow(ctx, `SELECT payload::text FROM audit_events WHERE action='recipient.deleted'`).Scan(&birth))
	assert.NotContains(t, birth, "1990", "в audit нет персональных данных")
}

// FR-O01: повод с бюджетом, датой и нумерологией (по умолчанию выключена, FR-N01).
func TestOccasion_FRO01(t *testing.T) {
	e := newEnv(t)
	giver := e.login("giver@gulder.local")
	id := e.createRecipient(giver, "Мама")
	r := e.do(http.MethodPost, "/api/v1/occasions", giver, map[string]any{
		"recipient_id": id, "type": "march8", "relationship": "mother", "date": "2027-03-08",
		"budget_min": 10000, "budget_max": 25000.5, "place": "home", "formality": 2, "emotion_goal": "gratitude",
	})
	require.Equal(t, http.StatusCreated, r.Status, string(r.Body))
	o := r.JSON(t)
	assert.Equal(t, 25000.5, o["budget_max"])
	assert.Equal(t, false, o["numerology_enabled"])
	assert.Equal(t, "2027-03-08", o["date"])

	got := e.do(http.MethodGet, fmt.Sprintf("/api/v1/occasions/%d", int64(o["id"].(float64))), giver, nil)
	assert.Equal(t, http.StatusOK, got.Status)

	r = e.do(http.MethodPost, "/api/v1/occasions", giver, map[string]any{"recipient_id": id, "type": "party", "budget_min": 5000, "budget_max": 1000})
	require.Equal(t, http.StatusUnprocessableEntity, r.Status)
	d := r.JSON(t)["error"].(map[string]any)["details"].(map[string]any)
	assert.Contains(t, d, "type")
	assert.Contains(t, d, "budget_min")

	list := e.do(http.MethodGet, fmt.Sprintf("/api/v1/recipients/%d/occasions", id), giver, nil).JSON(t)
	assert.Len(t, list["items"], 1)
}

// §12, FR-C03: построчный отчёт импорта; неизвестный род → taxon_confirmed=false.
func TestCatalogImport_FRC03(t *testing.T) {
	e := newEnv(t)
	florist := e.login("florist@gulder.local")

	flowers := "code,trade_name,genus,color,scent\n" +
		"abay_rose_blue,Роза синяя,Rosa,blue,low\n" +
		"abay_mystery,Неизвестный цветок,Mysteria,pink,none\n" +
		"rose_red,Перехват общего кода,Rosa,red,low\n" +
		",Без кода,Rosa,red,loud\n"
	r := e.upload("/api/v1/catalog/import?kind=flowers", florist, flowers)
	require.Equal(t, http.StatusOK, r.Status, string(r.Body))
	var rep struct {
		Imported, Updated int
		Errors            []struct {
			Line           int
			Field, Message string
		}
	}
	r.Decode(t, &rep)
	assert.Equal(t, 2, rep.Imported)
	lines := map[int][]string{}
	for _, er := range rep.Errors {
		lines[er.Line] = append(lines[er.Line], er.Field)
	}
	assert.Contains(t, lines, 4, "общий код нельзя перезаписать")
	assert.ElementsMatch(t, []string{"code", "scent"}, lines[5])

	var confirmed bool
	var taxon *int64
	require.NoError(t, e.store.Pool.QueryRow(context.Background(),
		`SELECT taxon_confirmed, taxon_id FROM flowers WHERE code='abay_mystery'`).Scan(&confirmed, &taxon))
	assert.False(t, confirmed)
	assert.Nil(t, taxon)

	bouquets := "sku,name,price,discount_price,discount_until,composition\n" +
		"AB-100,Синие розы,15000,12000,2027-01-01,abay_rose_blue:9;gypsophila:3\n" +
		"AB-101,С загадкой,9000,,,abay_mystery:5\n" +
		"AB-102,Чужой цветок,9000,,,nonexistent:5\n" +
		"AB-001,Классика обновлённая,33000,,,rose_red:25\n"
	r = e.upload("/api/v1/catalog/import?kind=bouquets", florist, bouquets)
	require.Equal(t, http.StatusOK, r.Status, string(r.Body))
	r.Decode(t, &rep)
	assert.Equal(t, 2, rep.Imported)
	assert.Equal(t, 1, rep.Updated)
	require.Len(t, rep.Errors, 1)
	assert.Equal(t, 4, rep.Errors[0].Line)
	assert.Contains(t, rep.Errors[0].Message, "nonexistent")

	// неопознанное растение видно флористу в списке на подтверждение и подтверждается выбором таксона
	r = e.do(http.MethodGet, "/api/v1/catalog/flowers?unconfirmed=true", florist, nil)
	require.Equal(t, http.StatusOK, r.Status)
	var fl struct {
		Items []struct {
			ID   int64
			Code string
		}
	}
	r.Decode(t, &fl)
	codes := []string{}
	var mysteryID int64
	for _, f := range fl.Items {
		codes = append(codes, f.Code)
		if f.Code == "abay_mystery" {
			mysteryID = f.ID
		}
	}
	assert.ElementsMatch(t, []string{"abay_mystery", "protea_king"}, codes)
	var rosaID int64
	require.NoError(t, e.store.Pool.QueryRow(context.Background(), `SELECT id FROM plant_taxa WHERE genus='Rosa'`).Scan(&rosaID))
	r = e.do(http.MethodPut, fmt.Sprintf("/api/v1/catalog/flowers/%d/taxon", mysteryID), florist, map[string]any{"taxon_id": rosaID})
	assert.Equal(t, http.StatusNoContent, r.Status, string(r.Body))

	// другой флорист не может подтвердить чужой цветок
	florist2 := e.login("florist2@gulder.local")
	r = e.do(http.MethodPut, fmt.Sprintf("/api/v1/catalog/flowers/%d/taxon", mysteryID), florist2, map[string]any{"taxon_id": rosaID})
	assert.Equal(t, http.StatusNotFound, r.Status)

	r = e.upload("/api/v1/catalog/import?kind=gifts", florist, "sku,name\nX,Y\n")
	assert.Equal(t, http.StatusBadRequest, r.Status)
	r = e.upload("/api/v1/catalog/import?kind=plants", florist, "sku\n")
	assert.Equal(t, http.StatusUnprocessableEntity, r.Status)

	var events int
	require.NoError(t, e.store.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_events WHERE action IN ('catalog.imported','flower.taxon_confirmed')`).Scan(&events))
	assert.Equal(t, 3, events)
}

// §13: флорист меняет только свой магазин; изменение цены создаёт новую запись и audit.
func TestAvailabilityUpdate_OwnShopOnly(t *testing.T) {
	e := newEnv(t)
	florist, florist2 := e.login("florist@gulder.local"), e.login("florist2@gulder.local")

	r := e.do(http.MethodGet, "/api/v1/catalog/bouquets", florist, nil)
	require.Equal(t, http.StatusOK, r.Status)
	var list struct {
		Items []struct {
			SKU          string
			Availability struct{ ID int64 }
			Offer        struct{ Price float64 }
			Items        []struct{ Code string }
		}
	}
	r.Decode(t, &list)
	require.NotEmpty(t, list.Items)
	for _, b := range list.Items {
		assert.True(t, strings.HasPrefix(b.SKU, "AB-"), "только свой магазин")
		assert.NotEmpty(t, b.Items)
	}
	avail := list.Items[0].Availability.ID
	path := fmt.Sprintf("/api/v1/availability/%d", avail)

	r = e.do(http.MethodPut, path, florist2, map[string]any{"in_stock": false})
	assert.Equal(t, http.StatusNotFound, r.Status, "чужой магазин")

	r = e.do(http.MethodPut, path, florist, map[string]any{"in_stock": true, "price": 30000, "discount_price": 31000})
	assert.Equal(t, http.StatusUnprocessableEntity, r.Status)

	r = e.do(http.MethodPut, path, florist, map[string]any{
		"in_stock": false, "quantity": 0, "price": 30000,
		"discount_price": 27000, "discount_until": "2026-12-31T00:00:00Z",
	})
	require.Equal(t, http.StatusNoContent, r.Status, string(r.Body))

	r = e.do(http.MethodGet, "/api/v1/catalog/bouquets", florist, nil)
	r.Decode(t, &list)
	assert.Equal(t, 30000.0, list.Items[0].Offer.Price)

	var n int
	require.NoError(t, e.store.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_events WHERE action='availability.updated' AND entity_id=$1`, avail).Scan(&n))
	assert.Equal(t, 1, n)
}

func TestShops_OpenNow(t *testing.T) {
	e := newEnv(t)
	r := e.do(http.MethodGet, "/api/v1/shops", "", nil)
	require.Equal(t, http.StatusOK, r.Status)
	var body struct {
		Items []struct {
			Code    string
			OpenNow bool `json:"open_now"`
		}
	}
	r.Decode(t, &body)
	require.Len(t, body.Items, 6)
	for _, s := range body.Items {
		assert.True(t, s.OpenNow, "понедельник 12:00 — все магазины открыты (%s)", s.Code)
	}
}

func TestP1EndpointsNotImplemented(t *testing.T) {
	e := newEnv(t)
	r := e.do(http.MethodPost, "/api/v1/orders", "", nil)
	assert.Equal(t, http.StatusNotImplemented, r.Status)
	assert.Equal(t, "not_implemented", r.JSON(t)["error"].(map[string]any)["code"])
}

func byKey(items []any) map[string]map[string]any {
	out := map[string]map[string]any{}
	for _, it := range items {
		m := it.(map[string]any)
		out[m["key"].(string)] = m
	}
	return out
}
