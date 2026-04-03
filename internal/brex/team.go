package brex

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// TeamUser is a subset of GET /v2/users and /v2/users/me fields.
type TeamUser struct {
	ID          string `json:"id"`
	FirstName   string `json:"first_name"`
	LastName    string `json:"last_name"`
	Email       string `json:"email"`
	Status      string `json:"status"`
	DisplayName string `json:"-"` // not from API; filled by FullName()
}

func (u *TeamUser) FullName() string {
	return strings.TrimSpace(strings.TrimSpace(u.FirstName) + " " + strings.TrimSpace(u.LastName))
}

type listUsersResponse struct {
	NextCursor string     `json:"next_cursor"`
	Items      []TeamUser `json:"items"`
}

// ListUsersParams controls GET /v2/users.
type ListUsersParams struct {
	Cursor           string
	Limit            int
	Email            string
	RemoteDisplayID  string
}

func (c *Client) GetMe(ctx context.Context) (*TeamUser, error) {
	var u TeamUser
	if err := c.doJSON(ctx, http.MethodGet, "/v2/users/me", nil, &u); err != nil {
		return nil, err
	}
	return &u, nil
}

// ListUsers calls GET /v2/users (single page).
func (c *Client) ListUsers(ctx context.Context, p ListUsersParams) ([]TeamUser, string, error) {
	q := url.Values{}
	if p.Cursor != "" {
		q.Set("cursor", p.Cursor)
	}
	if p.Limit > 0 {
		q.Set("limit", fmt.Sprintf("%d", p.Limit))
	}
	if strings.TrimSpace(p.Email) != "" {
		q.Set("email", strings.TrimSpace(p.Email))
	}
	if strings.TrimSpace(p.RemoteDisplayID) != "" {
		q.Set("remote_display_id", strings.TrimSpace(p.RemoteDisplayID))
	}
	rel := "/v2/users"
	if enc := q.Encode(); enc != "" {
		rel += "?" + enc
	}
	var out listUsersResponse
	if err := c.doJSON(ctx, http.MethodGet, rel, nil, &out); err != nil {
		return nil, "", err
	}
	return out.Items, out.NextCursor, nil
}

// ListAllUsers paginates GET /v2/users until next_cursor is empty or maxPages reached (0 = unlimited).
func (c *Client) ListAllUsers(ctx context.Context, pageLimit int, maxPages int) ([]TeamUser, error) {
	if pageLimit <= 0 {
		pageLimit = 1000
	}
	var all []TeamUser
	cursor := ""
	pages := 0
	for {
		items, next, err := c.ListUsers(ctx, ListUsersParams{
			Cursor: cursor,
			Limit:  pageLimit,
		})
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
		pages++
		if next == "" {
			break
		}
		if maxPages > 0 && pages >= maxPages {
			break
		}
		cursor = next
	}
	return all, nil
}

// FindUserIDByDisplayName matches "First Last" case-insensitively with Brex first_name + last_name.
func FindUserIDByDisplayName(users []TeamUser, displayName string) (string, error) {
	want := normName(displayName)
	if want == "" {
		return "", fmt.Errorf("brex: empty display name")
	}
	var matches []string
	for i := range users {
		u := &users[i]
		got := normName(u.FullName())
		if got == want {
			matches = append(matches, u.ID)
		}
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("brex: no user matched display name %q", strings.TrimSpace(displayName))
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("brex: multiple users matched %q (narrow with --user-email or --user-id)", strings.TrimSpace(displayName))
	}
}

func normName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.Join(strings.Fields(s), " ")
	return s
}
