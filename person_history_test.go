package agentsdk

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/airlockrun/agentsdk/wire"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCurrentPersonHistoryUsesInvocationProof(t *testing.T) {
	const runID = "11111111-1111-1111-1111-111111111111"
	const proof = "abababababababababababababababababababababababababababababababab"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/agent/session/person/messages" || r.Header.Get(wire.InvocationTokenHeader) != proof || r.Header.Get("X-Airlock-Run-ID") != runID || r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("unproved history callback")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["fromSeq"] != float64(10) || body["limit"] != float64(20) || body["completedOnly"] != true || body["userId"] != nil || body["agentId"] != nil {
			t.Errorf("query=%v", body)
		}
		_ = json.NewEncoder(w).Encode(PersonHistoryResponse{Scope: "person_same_agent", SequenceNamespace: "airlock_native", Messages: []PersonHistoryMessage{{Seq: 11, Content: "owned history"}}})
	}))
	defer server.Close()
	a := newAgentRegistrationState(Config{Description: "test"})
	a.client = newAirlockClient(server.URL, "test-token", server.Client())
	active := newRun(a, runID, "", "22222222-2222-2222-2222-222222222222", context.Background())
	active.invocationToken = proof
	got, err := CurrentPersonHistory(contextWithRun(context.Background(), active), PersonHistoryQuery{Limit: 20, FromSeq: 10, CompletedOnly: true})
	if err != nil || len(got.Messages) != 1 || got.Messages[0].Content != "owned history" {
		t.Fatalf("history=%+v err=%v", got, err)
	}
}
func TestCurrentPersonHistoryRequiresConversation(t *testing.T) {
	if _, err := CurrentPersonHistory(context.Background(), PersonHistoryQuery{}); !errors.Is(err, ErrCurrentConversationUnavailable) {
		t.Fatalf("err=%v", err)
	}
}
