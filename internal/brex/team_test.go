package brex

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFindUserIDByDisplayName(t *testing.T) {
	users := []TeamUser{
		{ID: "u1", FirstName: "Nate", LastName: "Sesti"},
		{ID: "u2", FirstName: "Jane", LastName: "Doe"},
	}
	id, err := FindUserIDByDisplayName(users, "nate sesti")
	if err != nil || id != "u1" {
		t.Fatalf("got %q err %v", id, err)
	}
	_, err = FindUserIDByDisplayName(users, "nate")
	if err == nil {
		t.Fatal("expected no match")
	}
}

func TestClient_GetMe(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/users/me" {
			t.Errorf("path %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(TeamUser{ID: "me", FirstName: "Nate", LastName: "Sesti", Email: "n@example.com"})
	}))
	t.Cleanup(srv.Close)

	c, _ := NewClient("bxt_x", srv.Client())
	c.WithBaseURL(srv.URL)

	u, err := c.GetMe(context.Background())
	if err != nil || u.ID != "me" || u.FullName() != "Nate Sesti" {
		t.Fatalf("got %+v err %v", u, err)
	}
}
