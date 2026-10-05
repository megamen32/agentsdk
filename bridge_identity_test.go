package agentsdk

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestResolveBridgeIdentities(t *testing.T) {
	wantRequest := BridgeIdentityLookup{BridgeID: "bridge-id", SenderID: 100, ChatID: -200, MemberIDs: []string{"member-id"}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/agent/bridge-identities/resolve" || r.Header.Get("Authorization") != "Bearer app-token" {
			t.Fatalf("unexpected request: %s %s auth=%q", r.Method, r.URL.Path, r.Header.Get("Authorization"))
		}
		var got BridgeIdentityLookup
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil || !reflect.DeepEqual(got, wantRequest) {
			t.Fatalf("lookup=%+v err=%v", got, err)
		}
		_, _ = io.WriteString(w, `{"platform":"telegram_userbot","senderUserId":"sender-user","recipients":[{"userId":"member-id","status":"resolved","platformUserId":"300"},{"userId":"missing-id","status":"missing_link"}]}`)
	}))
	defer srv.Close()
	a := &Agent{phase: agentRunning, client: newAirlockClient(srv.URL, "app-token", srv.Client())}
	got, err := a.ResolveBridgeIdentities(context.Background(), wantRequest)
	want := BridgeIdentityResult{Platform: "telegram_userbot", SenderUserID: "sender-user", Recipients: []BridgeIdentityRecipient{
		{UserID: "member-id", Status: BridgeIdentityResolved, PlatformUserID: "300"},
		{UserID: "missing-id", Status: BridgeIdentityMissing},
	}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("ResolveBridgeIdentities=%+v, %v; want %+v", got, err, want)
	}
}

func TestResolveBridgeIdentitiesRequiresRuntime(t *testing.T) {
	got, err := (&Agent{}).ResolveBridgeIdentities(context.Background(), BridgeIdentityLookup{})
	if err == nil || !reflect.DeepEqual(got, BridgeIdentityResult{}) {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}
