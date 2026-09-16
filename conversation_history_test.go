package agentsdk

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/airlockrun/agentsdk/wire"
	"github.com/airlockrun/sol/session"
)

func TestCurrentConversationMessagesUsesBoundInvocation(t *testing.T) {
	const runID = "11111111-1111-1111-1111-111111111111"
	const invocationToken = "abababababababababababababababababababababababababababababababab"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/agent/session/current/messages" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("authorization = %q", got)
		}
		if got := r.Header.Get("X-Airlock-Run-ID"); got != runID {
			t.Fatalf("run id = %q", got)
		}
		if got := r.Header.Get(wire.InvocationTokenHeader); got != invocationToken {
			t.Fatalf("invocation token = %q", got)
		}
		_ = json.NewEncoder(w).Encode(wire.SessionLoadResponse{Messages: []session.Message{{Role: "user", Content: "Review the plan"}}, Revision: "7"})
	}))
	defer server.Close()

	agent := newAgentRegistrationState(Config{Description: "test agent"})
	agent.client = newAirlockClient(server.URL, "test-token", server.Client())
	run := newRun(agent, runID, "", "22222222-2222-2222-2222-222222222222", context.Background())
	run.invocationToken = invocationToken

	messages, err := CurrentConversationMessages(contextWithRun(context.Background(), run))
	if err != nil {
		t.Fatalf("CurrentConversationMessages: %v", err)
	}
	if len(messages) != 1 || messages[0].Role != "user" || messages[0].Content != "Review the plan" {
		t.Fatalf("messages = %#v", messages)
	}
}

func TestCurrentConversationMessagesRejectsUnavailableConversation(t *testing.T) {
	if _, err := CurrentConversationMessages(context.Background()); !errors.Is(err, ErrCurrentConversationUnavailable) {
		t.Fatalf("unbound error = %v", err)
	}

	agent := newAgentRegistrationState(Config{Description: "test agent"})
	run := newRun(agent, "run-1", "", "", context.Background())
	if _, err := CurrentConversationMessages(contextWithRun(context.Background(), run)); !errors.Is(err, ErrCurrentConversationUnavailable) {
		t.Fatalf("background error = %v", err)
	}

	if strings.TrimSpace(run.conversationID) != "" {
		t.Fatalf("test setup has a conversation id")
	}
}
