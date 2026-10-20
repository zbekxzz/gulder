package httpapi_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zbekxzz/gulder/backend/internal/research"
)

func (e *env) count(sql string, args ...any) int {
	e.t.Helper()
	var n int
	require.NoError(e.t, e.store.Pool.QueryRow(context.Background(), sql, args...).Scan(&n))
	return n
}

// giveFeedback создаёт получателя, запуск и n отзывов.
func (e *env) giveFeedback(token string, n int, budget int) {
	e.t.Helper()
	id := e.createRecipient(token, "Для отзывов", knownNoRestrictions()...)
	run, _ := e.recommend(token, e.createOccasion(token, id, budget, nil))
	for i := 0; i < n; i++ {
		r := e.do(http.MethodPost, fmt.Sprintf("/api/v1/recommendations/%d/feedback", run.RunID), token,
			map[string]any{"bouquet_id": run.Items[i%len(run.Items)].BouquetID, "rating": 4, "comment": "секретный комментарий"})
		require.Equal(e.t, http.StatusNoContent, r.Status, string(r.Body))
	}
}

// AC-10: без согласия в research-таблицах нет записей; триггер БД не даёт создать связку без согласия.
func TestAC10_NoResearchRecordsWithoutConsent(t *testing.T) {
	e := newEnv(t)
	giver := e.login("giver@gulder.local")
	e.giveFeedback(giver, 3, 30000)

	for _, table := range []string{"research.research_consents", "research.research_subjects"} {
		assert.Zero(t, e.count("SELECT count(*) FROM "+table), table)
	}
	_, err := e.store.Pool.Exec(context.Background(), `INSERT INTO research.research_subjects (research_id, user_id)
		VALUES (gen_random_uuid(), (SELECT id FROM users WHERE email='giver@gulder.local'))`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no active consent")

	st := e.do(http.MethodGet, "/api/v1/research/consent", giver, nil).JSON(t)
	assert.Equal(t, false, st["active"], "RM-01: выключен по умолчанию")
	assert.Equal(t, research.ConsentVersion, st["current_version"])
	assert.NotEmpty(t, st["text"])
}

// RM-01–RM-03, RM-06: согласие с версией, research_id, экспорт без идентификаторов и текста,
// группы < 5 скрыты, отзыв в один шаг исключает из следующих экспортов.
func TestResearchConsentAndExport(t *testing.T) {
	e := newEnv(t)
	giver, manager := e.login("giver@gulder.local"), e.login("manager@gulder.local")

	r := e.do(http.MethodPost, "/api/v1/research/consent", giver, map[string]any{"consent_version": "research-consent-v0"})
	assert.Equal(t, http.StatusConflict, r.Status, "устаревшая версия текста")
	r = e.do(http.MethodPost, "/api/v1/research/consent", giver, map[string]any{"consent_version": research.ConsentVersion})
	require.Equal(t, http.StatusOK, r.Status, string(r.Body))
	assert.Equal(t, true, r.JSON(t)["active"])
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM research.research_subjects`))
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM research.research_consents WHERE consent_version = $1`, research.ConsentVersion))
	// повторное согласие ничего не дублирует
	require.Equal(t, http.StatusOK, e.do(http.MethodPost, "/api/v1/research/consent", giver, map[string]any{"consent_version": research.ConsentVersion}).Status)
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM research.research_consents`))

	e.giveFeedback(giver, 5, 30000)                          // группа birthday/15000-30000: 5 записей — попадает в экспорт
	e.giveFeedback(giver, 2, 50000)                          // группа birthday/>30000: 2 записи — скрывается
	e.giveFeedback(e.login("giver2@gulder.local"), 6, 30000) // без согласия — не попадает

	r = e.do(http.MethodPost, "/api/v1/research/export", manager, nil)
	require.Equal(t, http.StatusOK, r.Status, string(r.Body))
	var exp research.Export
	r.Decode(t, &exp)
	assert.Len(t, exp.Rows, 5)
	assert.Equal(t, 1, exp.SuppressedGroups)
	body := strings.ToLower(string(r.Body))
	for _, leak := range []string{"giver@", "секретный", "для отзывов", "allerg", "birth_date", "\"lat"} {
		assert.NotContains(t, body, leak)
	}
	for _, row := range exp.Rows {
		assert.Len(t, row.ResearchID, 36)
		assert.Equal(t, "15000-30000", row.BudgetBand)
	}
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM research.research_exports WHERE row_count = 5`))
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM audit_events WHERE action = 'research.exported'`))

	// RM-02: отзыв одним запросом — связка удалена, следующий экспорт пуст
	require.Equal(t, http.StatusNoContent, e.do(http.MethodDelete, "/api/v1/research/consent", giver, nil).Status)
	assert.Zero(t, e.count(`SELECT count(*) FROM research.research_subjects`))
	r = e.do(http.MethodPost, "/api/v1/research/export", manager, nil)
	r.Decode(t, &exp)
	assert.Empty(t, exp.Rows)
	assert.Equal(t, 2, e.count(`SELECT count(*) FROM audit_events WHERE action IN ('research.consent_granted','research.consent_revoked')`))

	dd := e.do(http.MethodGet, "/api/v1/research/data-dictionary", manager, nil).JSON(t)
	assert.Equal(t, research.DataDictionaryVersion, dd["version"])
	assert.Equal(t, http.StatusForbidden, e.do(http.MethodPost, "/api/v1/research/export", giver, nil).Status)
}

// RM-05: dashboard видит запуски оценки и сравнение на одном split.
func TestEvaluationEndpoints_RM05(t *testing.T) {
	e := newEnv(t)
	manager := e.login("manager@gulder.local")
	ctx := context.Background()
	var a, b int64
	require.NoError(t, e.store.Pool.QueryRow(ctx, `INSERT INTO research.experiment_runs (model_version, dataset_version, split_id, seed, metrics)
		VALUES ('baseline-0.1','draft-1','s1',42,'{"ndcg@5":{"mean":0.7}}') RETURNING id`).Scan(&a))
	require.NoError(t, e.store.Pool.QueryRow(ctx, `INSERT INTO research.experiment_runs (model_version, dataset_version, split_id, seed, metrics)
		VALUES ('baseline-0.1+platt','draft-1','s1',42,'{"ndcg@5":{"mean":0.7}}') RETURNING id`).Scan(&b))
	_, err := e.store.Pool.Exec(ctx, `INSERT INTO research.experiment_runs (model_version, dataset_version, split_id, seed, metrics)
		VALUES ('other','draft-1','s2',1,'{}')`)
	require.NoError(t, err)

	list := e.do(http.MethodGet, "/api/v1/research/evaluation", manager, nil).JSON(t)
	assert.Len(t, list["items"], 3)
	got := e.do(http.MethodGet, fmt.Sprintf("/api/v1/research/evaluation/%d", a), manager, nil).JSON(t)
	assert.Len(t, got["same_split"], 2)
	assert.Equal(t, http.StatusNotFound, e.do(http.MethodGet, "/api/v1/research/evaluation/999999", manager, nil).Status)
}

// RM-07: экспертная разметка пишется в проверенную версию, черновик остаётся для сравнения.
func TestExpertJudgements_RM07(t *testing.T) {
	e := newEnv(t)
	manager := e.login("manager@gulder.local")
	r := e.do(http.MethodGet, "/api/v1/admin/expert-judgements?dataset_version=draft-1", manager, nil)
	require.Equal(t, http.StatusOK, r.Status)
	var list struct {
		Items []struct {
			CaseID            string         `json:"case_id"`
			BouquetID         int64          `json:"bouquet_id"`
			RecipientSnapshot map[string]any `json:"recipient_snapshot"`
			OccasionSnapshot  map[string]any `json:"occasion_snapshot"`
		}
	}
	r.Decode(t, &list)
	require.GreaterOrEqual(t, len(list.Items), 100)
	first := list.Items[0]
	body := map[string]any{
		"dataset_version": "v1", "case_id": first.CaseID, "recipient_snapshot": first.RecipientSnapshot,
		"occasion_snapshot": first.OccasionSnapshot, "bouquet_id": first.BouquetID, "grade": 2, "reason": "Проверено флористом",
	}
	require.Equal(t, http.StatusCreated, e.do(http.MethodPost, "/api/v1/admin/expert-judgements", manager, body).Status)
	body["grade"] = 3
	require.Equal(t, http.StatusCreated, e.do(http.MethodPost, "/api/v1/admin/expert-judgements", manager, body).Status, "повторная оценка обновляет")
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM expert_judgements WHERE dataset_version='v1' AND grade=3`))
	assert.Positive(t, e.count(`SELECT count(*) FROM expert_judgements WHERE dataset_version='draft-1'`))

	body["dataset_version"] = "draft-2"
	assert.Equal(t, http.StatusUnprocessableEntity, e.do(http.MethodPost, "/api/v1/admin/expert-judgements", manager, body).Status)
	body["dataset_version"], body["is_unsafe"] = "v1", true
	assert.Equal(t, http.StatusUnprocessableEntity, e.do(http.MethodPost, "/api/v1/admin/expert-judgements", manager, body).Status)
}

// §18.5: удаление аккаунта удаляет получателей, атрибуты (включая дату рождения) и research-связку.
func TestDeleteAccountCascades(t *testing.T) {
	e := newEnv(t)
	r := e.do(http.MethodPost, "/api/v1/auth/register", "", map[string]string{"email": "leaving@example.com", "password": "long-password"})
	require.Equal(t, http.StatusCreated, r.Status)
	token := r.JSON(t)["access_token"].(string)
	id := e.createRecipient(token, "Мама", attr("birth_date", "1960-05-01", "know"))
	run, _ := e.recommend(token, e.createOccasion(token, id, 20000, nil))
	require.Equal(t, http.StatusNoContent, e.do(http.MethodPost, fmt.Sprintf("/api/v1/recommendations/%d/feedback", run.RunID), token, map[string]any{"rating": 5}).Status)
	require.Equal(t, http.StatusOK, e.do(http.MethodPost, "/api/v1/research/consent", token, map[string]any{"consent_version": research.ConsentVersion}).Status)

	require.Equal(t, http.StatusNoContent, e.do(http.MethodDelete, "/api/v1/me", token, nil).Status)
	assert.Zero(t, e.count(`SELECT count(*) FROM users WHERE email='leaving@example.com'`))
	assert.Zero(t, e.count(`SELECT count(*) FROM recipient_attributes WHERE recipient_id=$1`, id))
	assert.Zero(t, e.count(`SELECT count(*) FROM recommendation_runs WHERE id=$1`, run.RunID))
	assert.Zero(t, e.count(`SELECT count(*) FROM research.research_subjects`))
	assert.Zero(t, e.count(`SELECT count(*) FROM research.research_consents`))
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM audit_events WHERE action='account.deleted'`))
}
