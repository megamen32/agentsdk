package agentsdk

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"
)

// AddMember grants user access to an existing Airlock user, within the current
// agent and authenticated admin invocation. It never creates external identities.
func AddMember(ctx context.Context, userID string) error {
	return mutateMember(ctx, http.MethodPost, userID)
}

// RemoveMember revokes an individual grant. Airlock rejects removal when a
// shared group would continue to admit the user, or when the user owns the app.
func RemoveMember(ctx context.Context, userID string) error {
	return mutateMember(ctx, http.MethodDelete, userID)
}

func mutateMember(ctx context.Context, method, userID string) error {
	run := runFromContext(ctx)
	if run == nil || run.agent == nil {
		return errors.New("agentsdk: membership mutation requires an authenticated admin invocation")
	}
	id, err := uuid.Parse(userID)
	if err != nil || id == uuid.Nil {
		return errors.New("agentsdk: member must be an explicit Airlock user UUID")
	}
	return run.agent.client.doJSON(contextWithRun(ctx, run), method, "/api/agent/members", struct {
		UserID string `json:"userId"`
	}{id.String()}, nil)
}
