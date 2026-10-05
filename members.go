package agentsdk

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"github.com/airlockrun/agentsdk/wire"
)

// Member contains a current-app member's identity and highest effective access
// across actual direct and group-derived grants, including public grants.
// IDs and access snapshots do not grant authority or notification eligibility.
type Member struct {
	User   User
	Access Access
}

// ListMembersOptions selects a page of the current app's member directory.
type ListMembersOptions struct {
	// Limit is 1 through 1000, or zero for the host default of 100.
	Limit  int
	Cursor string
}

// MemberPage contains deduplicated current-app members and an opaque next-page cursor.
// An empty NextCursor indicates the final page.
type MemberPage struct {
	Members    []Member
	NextCursor string
}

// AgentMember is the compatibility view used by deployed pre-0.7 apps. New
// code should prefer MemberPage; this shape intentionally includes no email.
type AgentMember struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
	Role Access `json:"role"`
}

// ListAgentMembers returns the flat compatibility roster for deployed apps.
func (a *Agent) ListAgentMembers(ctx context.Context) ([]AgentMember, error) {
	if !a.runtimeAvailable() {
		return nil, a.runtimeUnavailable("ListAgentMembers")
	}
	var response struct {
		Members []AgentMember `json:"members"`
	}
	if err := a.client.doJSON(ctx, http.MethodGet, "/api/agent/members", nil, &response); err != nil {
		return nil, err
	}
	if response.Members == nil {
		return []AgentMember{}, nil
	}
	return response.Members, nil
}

// ListMembers returns a page of the current app's members with actual direct or
// group-derived grants, including public grants. Users with no grant are excluded.
// Users are deduplicated with their highest effective access (admin > user > public).
// Each user's PlatformMember is true. The request uses the current app credential
// and works in application-owned contexts without borrowing the app owner's identity.
func (a *Agent) ListMembers(ctx context.Context, opts ListMembersOptions) (MemberPage, error) {
	if !a.runtimeAvailable() {
		return MemberPage{}, a.runtimeUnavailable("ListMembers")
	}
	if opts.Limit < 0 || opts.Limit > 1000 {
		return MemberPage{}, errors.New("agentsdk: member list limit must be between 0 and 1000")
	}
	q := url.Values{}
	if opts.Limit > 0 {
		q.Set("limit", strconv.Itoa(opts.Limit))
	}
	if opts.Cursor != "" {
		q.Set("cursor", opts.Cursor)
	}
	path := "/api/agent/members"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var response wire.ListMembersResponse
	if err := a.client.doJSON(ctx, "GET", path, nil, &response); err != nil {
		return MemberPage{}, err
	}
	page := MemberPage{Members: make([]Member, 0, len(response.Members)), NextCursor: response.NextCursor}
	for _, member := range response.Members {
		page.Members = append(page.Members, Member{User: User(member.User), Access: Access(member.Access)})
	}
	return page, nil
}
