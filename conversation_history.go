package agentsdk

import (
	"context"
	"errors"
	"net/http"

	"github.com/airlockrun/agentsdk/wire"
	"github.com/airlockrun/sol/session"
)

// ErrCurrentConversationUnavailable means the caller is not executing inside
// an Airlock conversation-bound run. Background work must keep its own durable
// state instead of borrowing a user's chat transcript.
var ErrCurrentConversationUnavailable = errors.New("agentsdk: current conversation is unavailable")

// CurrentConversationMessages returns the session transcript bound to the
// active invocation. It deliberately accepts neither a conversation ID nor a
// user ID, so an application cannot use this API to enumerate another user's
// conversation. Airlock verifies the run and invocation proof server-side.
func CurrentConversationMessages(ctx context.Context) ([]session.Message, error) {
	run := runFromContext(ctx)
	if run == nil || run.agent == nil || run.conversationID == "" {
		return nil, ErrCurrentConversationUnavailable
	}

	var response wire.SessionLoadResponse
	if err := run.agent.client.doJSON(contextWithRun(ctx, run), http.MethodGet, "/api/agent/session/current/messages", nil, &response); err != nil {
		return nil, err
	}
	return response.Messages, nil
}
