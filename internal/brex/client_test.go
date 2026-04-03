package brex

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClient_ListExpenses(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.RequestURI()
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			t.Errorf("missing bearer: %q", auth)
		}
		if r.Method != http.MethodGet {
			t.Errorf("method %s", r.Method)
		}
		_ = json.NewEncoder(w).Encode(listExpensesResponse{
			NextCursor: "c2",
			Items: []Expense{
				{ID: "e1", Memo: "", Merchant: &Merchant{RawDescriptor: "COFFEE"}},
			},
		})
	}))
	t.Cleanup(srv.Close)

	c, err := NewClient("bxt_test", srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	c.WithBaseURL(srv.URL)

	items, next, err := c.ListExpenses(context.Background(), ListExpensesParams{
		Limit:  10,
		Expand: []string{"merchant", "user"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if next != "c2" || len(items) != 1 || items[0].ID != "e1" {
		t.Fatalf("unexpected: %+v next=%q", items, next)
	}
	if !strings.Contains(gotPath, "merchant") {
		t.Fatalf("expand not in path: %s", gotPath)
	}
}

func TestClient_GetExpense(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/expenses/exp-1" {
			t.Errorf("path %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(Expense{ID: "exp-1", Memo: "x"})
	}))
	t.Cleanup(srv.Close)

	c, _ := NewClient("bxt_test", srv.Client())
	c.WithBaseURL(srv.URL)

	exp, err := c.GetExpense(context.Background(), "exp-1", []string{"merchant"})
	if err != nil || exp.ID != "exp-1" || exp.Memo != "x" {
		t.Fatalf("got %+v err %v", exp, err)
	}
}

func TestClient_UpdateExpenseMemo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/expenses/card/exp-1" {
			t.Errorf("path %s", r.URL.Path)
		}
		if r.Method != http.MethodPut {
			t.Errorf("method %s", r.Method)
		}
		var m map[string]string
		_ = json.NewDecoder(r.Body).Decode(&m)
		if m["memo"] != "hello" {
			t.Fatalf("body %+v", m)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"exp-1","memo":"hello","updated_at":"2024-01-01T00:00:00Z"}`))
	}))
	t.Cleanup(srv.Close)

	c, err := NewClient("bxt_test", srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	c.WithBaseURL(srv.URL)

	if err := c.UpdateExpenseMemo(context.Background(), "exp-1", "hello"); err != nil {
		t.Fatal(err)
	}
}

func TestClient_ListAllExpenses_MaxPages(t *testing.T) {
	n := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		next := ""
		if n < 3 {
			next = "more"
		}
		_ = json.NewEncoder(w).Encode(listExpensesResponse{
			NextCursor: next,
			Items:      []Expense{{ID: "x"}},
		})
	}))
	t.Cleanup(srv.Close)

	c, _ := NewClient("bxt_test", srv.Client())
	c.WithBaseURL(srv.URL)

	all, err := c.ListAllExpenses(context.Background(), ListExpensesParams{Limit: 100}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 || n != 2 {
		t.Fatalf("all=%d calls=%d", len(all), n)
	}
}

func TestListExpensesQueryValues_TimeRFC3339(t *testing.T) {
	ts := time.Date(2023, 1, 15, 12, 0, 0, 0, time.UTC)
	q := listExpensesQueryValues(ListExpensesParams{
		PurchasedAfter:  &ts,
		PurchasedBefore: &ts,
	})
	enc := q.Encode()
	if !strings.Contains(enc, "purchased_at_start=") || !strings.Contains(enc, "purchased_at_end=") {
		t.Fatalf("q=%s", enc)
	}
}
