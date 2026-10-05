package agentsdk

import (
	"context"
	"net/http"
	"time"
)

// PersonHistoryQuery filters the authenticated person's interactive chats with
// this agent. Sequence bounds refer to Airlock native IDs, not legacy IDs.
type PersonHistoryQuery struct {
	Limit          int    `json:"limit,omitempty"`
	FromSeq        int64  `json:"fromSeq,omitempty"`
	ToSeq          int64  `json:"toSeq,omitempty"`
	SinceTime      string `json:"sinceTime,omitempty"`
	ConversationID string `json:"conversationId,omitempty"`
	CompletedOnly  bool   `json:"completedOnly,omitempty"`
}
type PersonHistoryMessage struct {
	ID             string    `json:"id"`
	Seq            int64     `json:"seq"`
	ConversationID string    `json:"conversationId"`
	RunID          string    `json:"runId,omitempty"`
	Role           string    `json:"role"`
	Content        string    `json:"content"`
	CreatedAt      time.Time `json:"createdAt"`
}
type PersonHistoryResponse struct {
	Scope             string                 `json:"scope"`
	SequenceNamespace string                 `json:"sequenceNamespace"`
	Messages          []PersonHistoryMessage `json:"messages"`
}

// CurrentPersonHistory has no user or agent selector. Airlock derives both
// from the admitted invocation and applies ownership before reading history.
func CurrentPersonHistory(ctx context.Context, query PersonHistoryQuery) (PersonHistoryResponse, error) {
	active := runFromContext(ctx)
	if active == nil || active.agent == nil || active.conversationID == "" {
		return PersonHistoryResponse{}, ErrCurrentConversationUnavailable
	}
	var result PersonHistoryResponse
	err := active.agent.client.doJSON(contextWithRun(ctx, active), http.MethodPost, "/api/agent/session/person/messages", query, &result)
	return result, err
}
