package agentsdk

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/airlockrun/agentsdk/wire"
)

func TestOutputToCurrentConversationBoundTarget(t *testing.T) {
	const runID = "11111111-1111-1111-1111-111111111111"
	const conversationID = "22222222-2222-2222-2222-222222222222"
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "POST" || r.URL.Path != "/api/agent/print" {
			t.Errorf("wrong route %s %s", r.Method, r.URL.Path)
		}
		var request wire.PrintRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if request.ConversationID != conversationID || request.RunID != runID || request.Topic != "" || request.UserID != "" {
			t.Errorf("wrong output target %#v", request)
		}
		if len(request.Parts) != 1 || request.Parts[0].Text != "Finished" {
			t.Errorf("wrong content %#v", request.Parts)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	agent := newAgentRegistrationState(Config{Description: "test"})
	agent.client = newAirlockClient(server.URL, "test-token", server.Client())
	run := newRun(agent, runID, "", conversationID, context.Background())
	err := OutputToCurrentConversation(contextWithRun(context.Background(), run), []DisplayPart{{Type: DisplayPartTypeText, Text: "Finished"}})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("calls=%d", calls)
	}
}

func TestOutputToCurrentConversationRejectsUnbound(t *testing.T) {
	if err := OutputToCurrentConversation(context.Background(), nil); !errors.Is(err, ErrCurrentConversationUnavailable) {
		t.Fatalf("error=%v", err)
	}
	agent := newAgentRegistrationState(Config{Description: "test"})
	run := newRun(agent, "run", "", "", context.Background())
	if err := OutputToCurrentConversation(contextWithRun(context.Background(), run), nil); !errors.Is(err, ErrCurrentConversationUnavailable) {
		t.Fatalf("error=%v", err)
	}
}

func TestOutputToCurrentConversationOnceCarriesStableKey(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request wire.PrintRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if request.IdempotencyKey != "completion-key" || request.Topic != "" || request.ConversationID != "22222222-2222-2222-2222-222222222222" {
			t.Errorf("request=%#v", request)
		}
		calls++
		w.WriteHeader(200)
	}))
	defer server.Close()
	agent := newAgentRegistrationState(Config{Description: "test"})
	agent.client = newAirlockClient(server.URL, "test", server.Client())
	run := newRun(agent, "11111111-1111-1111-1111-111111111111", "", "22222222-2222-2222-2222-222222222222", context.Background())
	ctx := contextWithRun(context.Background(), run)
	for i := 0; i < 2; i++ {
		if err := OutputToCurrentConversationOnce(ctx, "completion-key", []DisplayPart{{Type: DisplayPartTypeText, Text: "done"}}); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 2 {
		t.Fatalf("calls=%d", calls)
	}
	if err := OutputToCurrentConversationOnce(ctx, "", []DisplayPart{{Type: DisplayPartTypeText, Text: "done"}}); err == nil {
		t.Fatal("blank key accepted")
	}
	if err := OutputToCurrentConversationOnce(context.Background(), "completion-key", nil); !errors.Is(err, ErrCurrentConversationUnavailable) {
		t.Fatalf("unbound=%v", err)
	}
}
