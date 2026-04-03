package brex

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const DefaultBaseURL = "https://api.brex.com"

var ErrEmptyToken = errors.New("brex: token is empty")

// Client talks to the Brex Expenses API.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

func NewClient(token string, httpClient *http.Client) (*Client, error) {
	if strings.TrimSpace(token) == "" {
		return nil, ErrEmptyToken
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{
		baseURL:    DefaultBaseURL,
		token:      strings.TrimSpace(token),
		httpClient: httpClient,
	}, nil
}

func (c *Client) WithBaseURL(base string) *Client {
	if base != "" {
		c.baseURL = strings.TrimRight(base, "/")
	}
	return c
}

// Money is an amount in the smallest unit of currency (e.g. cents for USD).
type Money struct {
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

// Expense is a subset of Brex list/get expense fields used for memo workflows.
type Expense struct {
	ID               string    `json:"id"`
	Memo             string    `json:"memo"`
	ExpenseType      string    `json:"expense_type"`
	Status           string    `json:"status"`
	PaymentStatus    string    `json:"payment_status"`
	PurchasedAt      string    `json:"purchased_at"`
	Category         string    `json:"category"`
	DashboardURL     string    `json:"dashboard_url"`
	OriginalAmount   *Money    `json:"original_amount"`
	BillingAmount    *Money    `json:"billing_amount"`
	PurchasedAmount  *Money    `json:"purchased_amount"`
	Merchant         *Merchant `json:"merchant"`
	UserID           string    `json:"user_id"`
	User             *User     `json:"user"`
}

type Merchant struct {
	RawDescriptor string `json:"raw_descriptor"`
	MCC           string `json:"mcc"`
	Country       string `json:"country"`
}

type User struct {
	ID        string `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

type listExpensesResponse struct {
	NextCursor string    `json:"next_cursor"`
	Items      []Expense `json:"items"`
}

// ListExpensesParams controls GET /v1/expenses.
type ListExpensesParams struct {
	Cursor          string
	Limit           int
	UserIDs         []string
	Status          []string
	PaymentStatus   []string
	ExpenseType     []string
	PurchasedAfter  *time.Time
	PurchasedBefore *time.Time
	Expand          []string
}

func listExpensesQueryValues(p ListExpensesParams) url.Values {
	q := url.Values{}
	if p.Cursor != "" {
		q.Set("cursor", p.Cursor)
	}
	if p.Limit > 0 {
		q.Set("limit", fmt.Sprintf("%d", p.Limit))
	}
	for _, id := range p.UserIDs {
		q.Add("user_id[]", id)
	}
	for _, s := range p.Status {
		q.Add("status[]", s)
	}
	for _, s := range p.PaymentStatus {
		q.Add("payment_status[]", s)
	}
	for _, t := range p.ExpenseType {
		q.Add("expense_type[]", t)
	}
	if p.PurchasedAfter != nil {
		q.Set("purchased_at_start", p.PurchasedAfter.UTC().Format(time.RFC3339Nano))
	}
	if p.PurchasedBefore != nil {
		q.Set("purchased_at_end", p.PurchasedBefore.UTC().Format(time.RFC3339Nano))
	}
	for _, e := range p.Expand {
		q.Add("expand[]", e)
	}
	return q
}

func (c *Client) ListExpenses(ctx context.Context, p ListExpensesParams) ([]Expense, string, error) {
	q := listExpensesQueryValues(p)
	rel := "/v1/expenses"
	if enc := q.Encode(); enc != "" {
		rel += "?" + enc
	}
	var out listExpensesResponse
	if err := c.doJSON(ctx, http.MethodGet, rel, nil, &out); err != nil {
		return nil, "", err
	}
	return out.Items, out.NextCursor, nil
}

// ListAllExpenses paginates until next_cursor is empty or maxPages is reached (0 = unlimited).
func (c *Client) ListAllExpenses(ctx context.Context, p ListExpensesParams, maxPages int) ([]Expense, error) {
	var all []Expense
	cursor := p.Cursor
	pages := 0
	for {
		p.Cursor = cursor
		items, next, err := c.ListExpenses(ctx, p)
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

// GetExpense returns GET /v1/expenses/{id}.
func (c *Client) GetExpense(ctx context.Context, id string, expand []string) (*Expense, error) {
	q := url.Values{}
	for _, e := range expand {
		q.Add("expand[]", e)
	}
	rel := "/v1/expenses/" + url.PathEscape(id)
	if enc := q.Encode(); enc != "" {
		rel += "?" + enc
	}
	var exp Expense
	if err := c.doJSON(ctx, http.MethodGet, rel, nil, &exp); err != nil {
		return nil, err
	}
	return &exp, nil
}

// UpdateExpenseMemo sets the memo on a card expense via PUT /v1/expenses/card/{expense_id}.
func (c *Client) UpdateExpenseMemo(ctx context.Context, expenseID, memo string) error {
	rel := "/v1/expenses/card/" + url.PathEscape(expenseID)
	body := map[string]string{"memo": memo}
	var discard json.RawMessage
	return c.doJSON(ctx, http.MethodPut, rel, body, &discard)
}

func (c *Client) doJSON(ctx context.Context, method, path string, reqBody any, respBody any) error {
	u := c.baseURL + path
	var r io.Reader
	if reqBody != nil {
		b, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("brex: encode body: %w", err)
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, r)
	if err != nil {
		return fmt.Errorf("brex: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("brex: request: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("brex: %s %s -> %s: %s", method, path, resp.Status, strings.TrimSpace(string(raw)))
	}
	if len(raw) == 0 || respBody == nil {
		return nil
	}
	if err := json.Unmarshal(raw, respBody); err != nil {
		return fmt.Errorf("brex: decode response: %w", err)
	}
	return nil
}
