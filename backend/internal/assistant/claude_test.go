package assistant

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zbekxzz/gulder/backend/internal/domain"
)

func fakeAnthropic(t *testing.T, stopReason, text string, captured *map[string]any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(body, captured))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "msg_test", "type": "message", "role": "assistant", "model": "claude-opus-5-5",
			"content":     []map[string]any{{"type": "text", "text": text}},
			"stop_reason": stopReason, "usage": map[string]any{"input_tokens": 10, "output_tokens": 10},
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestClaudeProposeStructuredOutput(t *testing.T) {
	var req map[string]any
	srv := fakeAnthropic(t, "end_turn", `{"sets":[{"bouquet_id":1,"gift_ids":[101],"reasons":["Нежно"],"title":"Классика"}]}`, &req)
	c := NewClaude("test-key", "claude-opus-5-5", 5*time.Second, option.WithBaseURL(srv.URL))

	p, err := c.Propose(context.Background(), Prompt{
		OccasionType: "birthday", Budget: domain.Tenge(20000),
		Bouquets: []BouquetCandidate{{ID: 1, Name: "Розы", Price: domain.Tenge(12000)}},
	})
	require.NoError(t, err)
	require.Len(t, p.Sets, 1)
	assert.Equal(t, int64(1), p.Sets[0].BouquetID)
	assert.Equal(t, []int64{101}, p.Sets[0].GiftIDs)

	assert.Equal(t, "claude-opus-5-5", req["model"])
	format := req["output_config"].(map[string]any)["format"].(map[string]any)
	assert.Equal(t, "json_schema", format["type"])
	assert.Equal(t, "default", req["fallbacks"])
	assert.NotContains(t, req, "tool_choice")
	// в промпт уходят только позиции каталога и повод — без профиля получателя
	msg := req["messages"].([]any)[0].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string)
	assert.Contains(t, msg, `"occasion_type":"birthday"`)
	assert.NotContains(t, msg, "allerg")
}

func TestClaudeRefusalAndBadJSON(t *testing.T) {
	var req map[string]any
	srv := fakeAnthropic(t, "refusal", "", &req)
	_, err := NewClaude("k", "claude-opus-5-5", time.Second, option.WithBaseURL(srv.URL)).Propose(context.Background(), Prompt{})
	assert.ErrorIs(t, err, ErrRefused)

	srv2 := fakeAnthropic(t, "end_turn", "не json", &req)
	_, err = NewClaude("k", "claude-opus-5-5", time.Second, option.WithBaseURL(srv2.URL)).Propose(context.Background(), Prompt{})
	assert.Error(t, err)
}
