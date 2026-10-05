package agentsdk

import (
	"context"
	"net/http"
)

// BridgeIdentityLookup is an exact platform lookup for one admitted bridge
// event and an explicit set of current-app member UUIDs.
type BridgeIdentityLookup struct {
	BridgeID  string   `json:"bridgeId"`
	SenderID  int64    `json:"senderId"`
	ChatID    int64    `json:"chatId"`
	MemberIDs []string `json:"memberIds"`
}

type BridgeIdentityStatus string

const (
	BridgeIdentityResolved  BridgeIdentityStatus = "resolved"
	BridgeIdentityMissing   BridgeIdentityStatus = "missing_link"
	BridgeIdentityAmbiguous BridgeIdentityStatus = "ambiguous_link"
	BridgeIdentityNotMember BridgeIdentityStatus = "not_member"
)

type BridgeIdentityRecipient struct {
	UserID         string               `json:"userId"`
	Status         BridgeIdentityStatus `json:"status"`
	PlatformUserID string               `json:"platformUserId,omitempty"`
}

type BridgeIdentityResult struct {
	Platform     string                    `json:"platform"`
	SenderUserID string                    `json:"senderUserId"`
	Recipients   []BridgeIdentityRecipient `json:"recipients"`
}

// ResolveBridgeIdentities delegates identity resolution to Airlock. The host
// re-admits the exact numeric sender/chat against the active bridge, verifies
// that bridge belongs to this app, and rechecks every supplied UUID against
// the app's live grants. The SDK never reads a tenant-wide identity directory.
func (a *Agent) ResolveBridgeIdentities(ctx context.Context, lookup BridgeIdentityLookup) (BridgeIdentityResult, error) {
	if !a.runtimeAvailable() {
		return BridgeIdentityResult{}, a.runtimeUnavailable("ResolveBridgeIdentities")
	}
	var result BridgeIdentityResult
	if err := a.client.doJSON(ctx, http.MethodPost, "/api/agent/bridge-identities/resolve", lookup, &result); err != nil {
		return BridgeIdentityResult{}, err
	}
	if result.Recipients == nil {
		result.Recipients = []BridgeIdentityRecipient{}
	}
	return result, nil
}
