package agentsdk

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSetCurrentUserTextModelUsesBoundInvocation(t *testing.T) {
	const runID = "11111111-1111-1111-1111-111111111111"
	const invocationToken = "abababababababababababababababababababababababababababababababab"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/agent/model-preference" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("X-Airlock-Run-ID"); got != runID {
			t.Fatalf("X-Airlock-Run-ID = %q", got)
		}
		if got := r.Header.Get("X-Airlock-Invocation-Token"); got != invocationToken {
			t.Fatalf("X-Airlock-Invocation-Token = %q", got)
		}
		var request setCurrentUserTextModelRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.Model != "minimax/MiniMax-M2.7" {
			t.Fatalf("model = %q", request.Model)
		}
		_ = json.NewEncoder(w).Encode(CurrentUserTextModel{Model: request.Model, ProviderID: "provider-1"})
	}))
	defer server.Close()

	agent := newAgentRegistrationState(Config{Description: "test agent"})
	agent.client = newAirlockClient(server.URL, "test-token", server.Client())
	run := newRun(agent, runID, "", "", context.Background())
	run.invocationToken = invocationToken

	got, err := SetCurrentUserTextModel(contextWithRun(context.Background(), run), "  minimax/MiniMax-M2.7  ")
	if err != nil {
		t.Fatalf("SetCurrentUserTextModel: %v", err)
	}
	if got.Model != "minimax/MiniMax-M2.7" || got.ProviderID != "provider-1" {
		t.Fatalf("response = %#v", got)
	}
}

func TestSetCurrentUserTextModelRejectsUnboundCall(t *testing.T) {
	if _, err := SetCurrentUserTextModel(context.Background(), "minimax/MiniMax-M2.7"); !errors.Is(err, ErrCurrentUserModelUnavailable) {
		t.Fatalf("unbound error = %v", err)
	}
}
