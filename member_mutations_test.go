package agentsdk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMemberMutationRequiresBoundInvocation(t *testing.T) {
	for _, call := range []func(context.Context, string) error{AddMember, RemoveMember} {
		if err := call(context.Background(), "11111111-1111-1111-1111-111111111111"); err == nil {
			t.Fatal("unbound mutation accepted")
		}
	}
}
func TestMemberMutationUsesInvocationAndNoAppSelector(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != method || r.URL.Path != "/api/agent/members" {
					t.Fatalf("request %s %s", r.Method, r.URL.Path)
				}
				if r.Header.Get("X-Airlock-Run-ID") != "run-id" || r.Header.Get("X-Airlock-Invocation-Token") != "invocation-secret" {
					t.Fatal("missing invocation binding")
				}
				var payload map[string]string
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Fatal(err)
				}
				if len(payload) != 1 || payload["userId"] != "22222222-2222-2222-2222-222222222222" {
					t.Fatalf("payload %#v", payload)
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			defer server.Close()
			a := newAgentRegistrationState(Config{Description: "membership test"})
			a.client = newAirlockClient(server.URL, "app-token", server.Client())
			run := newRun(a, "run-id", "", "", context.Background())
			run.invocationToken = "invocation-secret"
			for _, invalid := range []string{"", "@telegram", "00000000-0000-0000-0000-000000000000"} {
				if err := mutateMember(contextWithRun(context.Background(), run), method, invalid); err == nil {
					t.Fatalf("invalid identity %q accepted", invalid)
				}
			}
			if err := mutateMember(contextWithRun(context.Background(), run), method, "22222222-2222-2222-2222-222222222222"); err != nil {
				t.Fatal(err)
			}
		})
	}
}
