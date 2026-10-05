package agentsdk

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

// ErrCurrentUserModelUnavailable means a model preference was requested outside
// an invocation-bound Airlock run. Applications must not select a model for an
// arbitrary user or agent.
var ErrCurrentUserModelUnavailable = errors.New("agentsdk: current user model preference is unavailable")

// CurrentUserTextModel is the model preference accepted for the caller bound to
// the current invocation. It takes effect on the caller's next text turn.
type CurrentUserTextModel struct {
	Model      string `json:"model"`
	ProviderID string `json:"providerId"`
}

type setCurrentUserTextModelRequest struct {
	Model string `json:"model"`
}

// SetCurrentUserTextModel records a text-model preference for exactly the
// current caller. It accepts no user ID or agent ID: Airlock resolves both from
// the authenticated run and verifies model entitlement server-side.
func SetCurrentUserTextModel(ctx context.Context, model string) (CurrentUserTextModel, error) {
	run := runFromContext(ctx)
	if run == nil || run.agent == nil {
		return CurrentUserTextModel{}, ErrCurrentUserModelUnavailable
	}
	model = strings.TrimSpace(model)
	if model == "" || len(model) > 120 {
		return CurrentUserTextModel{}, errors.New("agentsdk: model must contain 1..120 characters")
	}
	var response CurrentUserTextModel
	if err := run.agent.client.doJSON(contextWithRun(ctx, run), http.MethodPost, "/api/agent/model-preference", setCurrentUserTextModelRequest{Model: model}, &response); err != nil {
		return CurrentUserTextModel{}, err
	}
	return response, nil
}
