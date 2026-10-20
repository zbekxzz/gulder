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

func (e *env) id(sql string, args ...any) int64 {
	e.t.Helper()
	var id int64
	require.NoError(e.t, e.store.Pool.QueryRow(context.Background(), sql, args...).Scan(&id))
	return id
}

// AC-3 (API): опасная замена блокируется — 409, запись substitutions и audit substitution.blocked.
func TestAC3_DangerousSubstitutionBlocked_API(t *testing.T) {
	e := newEnv(t)
	giver, florist := e.login("giver@gulder.local"), e.login("florist@gulder.local")
	catAttrs := []map[string]any{
		attr("allergies", []string{}, "know"), attr("pets", []string{"cat"}, "know"),
		attr("asthma", false, "know"), attr("scent_sensitivity", "none", "know"),
	}
	occ := e.createOccasion(giver, e.createRecipient(giver, "Кошатница", catAttrs...), 40000, nil)

	bouquet := e.id(`SELECT id FROM bouquets WHERE sku='AB-001'`)
	rose := e.id(`SELECT id FROM flowers WHERE code='rose_red'`)
	lily := e.id(`SELECT id FROM flowers WHERE code='lily_white'`)
	gerbera := e.id(`SELECT id FROM flowers WHERE code='gerbera_pink'`)
	path := fmt.Sprintf("/api/v1/bouquets/%d/substitutions/validate", bouquet)

	// мгновенная проверка (dry run): 409, но без записи
	r := e.do(http.MethodPost, path, florist, map[string]any{"from_flower_id": rose, "to_flower_id": lily, "occasion_id": occ, "dry_run": true})
	require.Equal(t, http.StatusConflict, r.Status, string(r.Body))
	errBody := r.JSON(t)["error"].(map[string]any)
	assert.Equal(t, "substitution_blocked", errBody["code"])
	gv := errBody["details"].(map[string]any)["giver_view"].(map[string]any)
	assert.Equal(t, false, gv["suitable"])

	// применение: 409 + журнал + audit
	r = e.do(http.MethodPost, path, florist, map[string]any{"from_flower_id": rose, "to_flower_id": lily, "occasion_id": occ})
	require.Equal(t, http.StatusConflict, r.Status)
	var subs, events int
	require.NoError(t, e.store.Pool.QueryRow(context.Background(), `SELECT count(*) FROM substitutions WHERE status='blocked'`).Scan(&subs))
	require.NoError(t, e.store.Pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_events WHERE action='substitution.blocked'`).Scan(&events))
	assert.Equal(t, 1, subs, "dry run не пишет журнал")
	assert.Equal(t, 1, events)

	// безопасная замена: 200, новый состав, audit approved
	r = e.do(http.MethodPost, path, florist, map[string]any{"from_flower_id": rose, "to_flower_id": gerbera, "occasion_id": occ})
	require.Equal(t, http.StatusOK, r.Status, string(r.Body))
	ok := r.JSON(t)
	assert.Equal(t, "allow", ok["decision"])
	assert.Len(t, ok["composition"], 1)
	require.NoError(t, e.store.Pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_events WHERE action='substitution.approved'`).Scan(&events))
	assert.Equal(t, 1, events)

	// даритель проверяет замену для своего повода тем же API
	r = e.do(http.MethodPost, path, giver, map[string]any{"from_flower_id": rose, "to_flower_id": lily, "occasion_id": occ, "dry_run": true})
	assert.Equal(t, http.StatusConflict, r.Status)
	// неопознанное растение блокируется и без повода (FR-C03)
	protea := e.id(`SELECT id FROM flowers WHERE code='protea_king'`)
	r = e.do(http.MethodPost, path, florist, map[string]any{"from_flower_id": rose, "to_flower_id": protea, "dry_run": true})
	assert.Equal(t, http.StatusConflict, r.Status)

	// флорист другого магазина букет не видит; цветка нет в букете → 422
	assert.Equal(t, http.StatusNotFound, e.do(http.MethodPost, path, e.login("florist2@gulder.local"), map[string]any{"from_flower_id": rose, "to_flower_id": gerbera}).Status)
	assert.Equal(t, http.StatusUnprocessableEntity, e.do(http.MethodPost, path, florist, map[string]any{"from_flower_id": lily, "to_flower_id": gerbera}).Status)
	assert.Equal(t, http.StatusForbidden, e.do(http.MethodPost, path, e.login("manager@gulder.local"), map[string]any{"from_flower_id": rose, "to_flower_id": gerbera}).Status)
}

// Флорист видит причину блокировки только через GiverView: ограничения, указанные получателем, скрыты.
func TestSubstitutionHidesRecipientRestrictions(t *testing.T) {
	e := newEnv(t)
	giver, florist := e.login("giver@gulder.local"), e.login("florist@gulder.local")
	rec := e.createRecipient(giver, "Тайна", attr("pets", []string{}, "know"), attr("asthma", false, "know"), attr("scent_sensitivity", "none", "know"))
	_, err := e.store.Pool.Exec(context.Background(), `INSERT INTO recipient_attributes (recipient_id, key, value, source, confidence)
		VALUES ($1,'allergies','["genus:lilium"]','recipient','know')`, rec)
	require.NoError(t, err)
	occ := e.createOccasion(giver, rec, 40000, nil)
	path := fmt.Sprintf("/api/v1/bouquets/%d/substitutions/validate", e.id(`SELECT id FROM bouquets WHERE sku='AB-001'`))
	r := e.do(http.MethodPost, path, florist, map[string]any{
		"from_flower_id": e.id(`SELECT id FROM flowers WHERE code='rose_red'`), "to_flower_id": e.id(`SELECT id FROM flowers WHERE code='lily_white'`),
		"occasion_id": occ, "dry_run": true,
	})
	require.Equal(t, http.StatusConflict, r.Status)
	body := strings.ToLower(string(r.Body))
	assert.NotContains(t, body, "lilium")
	assert.NotContains(t, body, "аллерги")
	assert.Contains(t, body, "recipient_restriction")
}

// AC-8: после feedback меняется только профиль соответствующего получателя.
func TestAC8_FeedbackChangesOnlyThatRecipient(t *testing.T) {
	e := newEnv(t)
	giver := e.login("giver@gulder.local")
	a := e.createRecipient(giver, "Первый", knownNoRestrictions()...)
	b := e.createRecipient(giver, "Второй", knownNoRestrictions()...)
	runA, _ := e.recommend(giver, e.createOccasion(giver, a, 30000, nil))
	e.recommend(giver, e.createOccasion(giver, b, 30000, nil))

	snapshot := func(id int64) string {
		var s string
		require.NoError(t, e.store.Pool.QueryRow(context.Background(),
			`SELECT coalesce(string_agg(key || '=' || coalesce(value::text,'null') || '/' || source || '/' || confidence, ';' ORDER BY key), '')
			   FROM recipient_attributes WHERE recipient_id = $1`, id).Scan(&s))
		return s
	}
	beforeB := snapshot(b)
	beforeA := snapshot(a)

	liked := runA.Items[0].BouquetID
	r := e.do(http.MethodPost, fmt.Sprintf("/api/v1/recommendations/%d/feedback", runA.RunID), giver,
		map[string]any{"bouquet_id": liked, "rating": 5, "liked": true, "comment": "Очень понравилось"})
	require.Equal(t, http.StatusNoContent, r.Status, string(r.Body))

	assert.Equal(t, beforeB, snapshot(b), "профиль второго получателя не изменился")
	assert.NotEqual(t, beforeA, snapshot(a))
	got := byKey(e.do(http.MethodGet, fmt.Sprintf("/api/v1/recipients/%d", a), giver, nil).JSON(t)["attributes"].([]any))
	assert.Equal(t, "feedback", got["liked_flowers"]["source"])
	assert.Equal(t, "assume", got["liked_flowers"]["confidence"])
	assert.NotEmpty(t, got["liked_flowers"]["value"])

	// поле, указанное дарителем, отзыв не перезаписывает
	r = e.do(http.MethodPut, fmt.Sprintf("/api/v1/recipients/%d", a), giver, map[string]any{"attributes": []any{attr("disliked_colors", []string{"red"}, "know")}})
	require.Equal(t, http.StatusOK, r.Status)
	r = e.do(http.MethodPost, fmt.Sprintf("/api/v1/recommendations/%d/feedback", runA.RunID), giver, map[string]any{"bouquet_id": liked, "liked": false})
	require.Equal(t, http.StatusNoContent, r.Status)
	got = byKey(e.do(http.MethodGet, fmt.Sprintf("/api/v1/recipients/%d", a), giver, nil).JSON(t)["attributes"].([]any))
	assert.Equal(t, []any{"red"}, got["disliked_colors"]["value"])
	assert.Equal(t, "giver", got["disliked_colors"]["source"])

	assert.Equal(t, http.StatusNotFound, e.do(http.MethodPost, fmt.Sprintf("/api/v1/recommendations/%d/feedback", runA.RunID),
		e.login("giver2@gulder.local"), map[string]any{"rating": 1}).Status)
	assert.Equal(t, http.StatusUnprocessableEntity, e.do(http.MethodPost, fmt.Sprintf("/api/v1/recommendations/%d/feedback", runA.RunID),
		giver, map[string]any{}).Status)
}

func TestManagerScreensData(t *testing.T) {
	e := newEnv(t)
	manager := e.login("manager@gulder.local")
	dq := e.do(http.MethodGet, "/api/v1/admin/data-quality", manager, nil)
	require.Equal(t, http.StatusOK, dq.Status)
	body := dq.JSON(t)
	assert.Empty(t, body["taxa_without_aspca"], "M7: у всех строк есть ссылка ASPCA")
	assert.Len(t, body["taxa_unverified"], 1, "спорная гипсофила ждёт проверки человеком")
	assert.Len(t, body["unconfirmed_flowers"], 2)
	assert.NotEmpty(t, body["gifts_without_composition"])

	an := e.do(http.MethodGet, "/api/v1/admin/analytics", manager, nil)
	require.Equal(t, http.StatusOK, an.Status, string(an.Body))
	assert.Contains(t, an.JSON(t), "exclusion_codes")
	rl := e.do(http.MethodGet, "/api/v1/admin/rules", manager, nil).JSON(t)
	assert.Equal(t, "rules-1.0.0", rl["rules_version"])
	assert.Equal(t, http.StatusForbidden, e.do(http.MethodGet, "/api/v1/admin/analytics", e.login("giver@gulder.local"), nil).Status)
}
