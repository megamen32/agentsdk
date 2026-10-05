package agentsdk

import (
	"context"
	"errors"
	"github.com/airlockrun/agentsdk/wire"
	"strings"
)

// OutputToCurrentConversation delivers display parts only to the conversation
// bound to this invocation (including a user job's originating conversation).
// It accepts no conversation or user selector and never broadcasts to topics.
func OutputToCurrentConversation(ctx context.Context, parts []DisplayPart) error {
	r := runFromContext(ctx)
	if r == nil || r.agent == nil || r.conversationID == "" {
		return ErrCurrentConversationUnavailable
	}
	return r.output(contextWithRun(ctx, r), parts, "")
}

// OutputToCurrentConversationOnce persists a text completion once per key in
// the bound native conversation. Retrying an identical key and payload is safe;
// reusing a key for different content is rejected. Bridge delivery is unsupported.
func OutputToCurrentConversationOnce(ctx context.Context, key string, parts []DisplayPart) error {
	r := runFromContext(ctx)
	if r == nil || r.agent == nil || r.conversationID == "" {
		return ErrCurrentConversationUnavailable
	}
	if strings.TrimSpace(key) == "" || len(key) > 128 {
		return errors.New("agentsdk: output idempotency key requires 1 to 128 bytes")
	}
	for _, part := range parts {
		if part.Type != DisplayPartTypeText || part.Source != "" || len(part.Data) > 0 {
			return errors.New("agentsdk: idempotent output supports text only")
		}
	}
	return r.agent.client.doJSON(contextWithRun(ctx, r), "POST", "/api/agent/print", wire.PrintRequest{Parts: toWireDisplayParts(parts), ConversationID: r.conversationID, RunID: r.id, IdempotencyKey: key}, nil)
}
