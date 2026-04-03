package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/sestinj/wt-cycle/internal/brex"
	"github.com/spf13/cobra"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	var token, baseURL string
	root := &cobra.Command{
		Use:   "brex-memo",
		Short: "List Brex expenses missing memos and apply memos in bulk",
		Long: strings.TrimSpace(`
Automates memo workflows against the Brex Expenses API.

Set BREX_TOKEN to your dashboard user token (Developer → Settings → Create Token).
Include scopes that allow reading expenses and updating card expenses.

Example:
  export BREX_TOKEN=bxt_...
  brex-memo verify
  brex-memo export --user-id cuuser_... --output memos.csv
  # edit the memo column, then:
  brex-memo apply --csv memos.csv --dry-run
  brex-memo apply --csv memos.csv`),
		SilenceUsage: true,
	}
	root.PersistentFlags().StringVar(&token, "token", os.Getenv("BREX_TOKEN"), "Brex user token (or env BREX_TOKEN)")
	root.PersistentFlags().StringVar(&baseURL, "base-url", brex.DefaultBaseURL, "API base URL (override for tests)")

	root.AddCommand(
		verifyCmd(&token, &baseURL),
		listCmd(&token, &baseURL),
		exportCmd(&token, &baseURL),
		applyCmd(&token, &baseURL),
	)
	return root
}

func clientFromFlags(token, baseURL string) (*brex.Client, error) {
	c, err := brex.NewClient(token, nil)
	if err != nil {
		return nil, err
	}
	return c.WithBaseURL(baseURL), nil
}

func verifyCmd(token, baseURL *string) *cobra.Command {
	var checkWrite bool
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Call the API and print whether read access works",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			c, err := clientFromFlags(*token, *baseURL)
			if err != nil {
				return err
			}
			items, _, err := c.ListExpenses(ctx, brex.ListExpensesParams{
				Limit:       1,
				Expand:      []string{"merchant"},
				ExpenseType: []string{"CARD"},
			})
			if err != nil {
				return fmt.Errorf("list expenses: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "OK: read access works (%d expense(s) in first page).\n", len(items))
			if len(items) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "sample id: %s\n", items[0].ID)
			}
			if !checkWrite {
				return nil
			}
			if len(items) == 0 {
				return errors.New("cannot check write: no CARD expenses returned (narrow filters or add expenses)")
			}
			id := items[0].ID
			full, err := c.GetExpense(ctx, id, nil)
			if err != nil {
				return fmt.Errorf("get expense %s: %w", id, err)
			}
			memo := strings.TrimSpace(full.Memo)
			if err := c.UpdateExpenseMemo(ctx, id, memo); err != nil {
				return fmt.Errorf("write check (PUT memo): %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "OK: write access works (re-stored memo on %s).\n", id)
			return nil
		},
	}
	cmd.Flags().BoolVar(&checkWrite, "check-write", false, "also PUT the same memo on a sample CARD expense (safe no-op if memo empty)")
	return cmd
}

func listCmd(token, baseURL *string) *cobra.Command {
	var jsonOut, cardOnly bool
	var maxPages int
	var userIDs []string
	var userEmail, userName string
	var filterMe bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Print expenses with an empty memo",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			c, err := clientFromFlags(*token, *baseURL)
			if err != nil {
				return err
			}
			uid, err := resolveExpenseUserIDs(ctx, c, userIDs, userEmail, userName, filterMe)
			if err != nil {
				return err
			}
			params := brex.ListExpensesParams{
				Limit:   100,
				Expand:  []string{"merchant", "user"},
				UserIDs: uid,
			}
			if cardOnly {
				params.ExpenseType = []string{"CARD"}
			}
			all, err := c.ListAllExpenses(ctx, params, maxPages)
			if err != nil {
				return err
			}
			missing := filterMissingMemo(all, cardOnly)
			if jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(missing)
			}
			for _, e := range missing {
				line := formatExpenseLine(e)
				fmt.Fprintln(cmd.OutOrStdout(), line)
			}
			fmt.Fprintf(cmd.OutOrStderr(), "%d expense(s) missing a memo.\n", len(missing))
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "JSON array output")
	cmd.Flags().BoolVar(&cardOnly, "card-only", true, "only CARD expenses (required for card memo updates)")
	cmd.Flags().IntVar(&maxPages, "max-pages", 0, "max list pages (100 expenses each; 0 = all)")
	cmd.Flags().StringSliceVar(&userIDs, "user-id", nil, "only expenses for Brex user id(s); repeat or comma-separated (dashboard filter decodes to cuuser_…)")
	cmd.Flags().StringVar(&userEmail, "user-email", "", "resolve user id from email (needs users.readonly scope)")
	cmd.Flags().StringVar(&userName, "user-name", "", "resolve user id from full name e.g. 'Nate Sesti' (needs users.readonly; lists users)")
	cmd.Flags().BoolVar(&filterMe, "me", false, "only expenses for the authenticated user")
	return cmd
}

func exportCmd(token, baseURL *string) *cobra.Command {
	var outPath string
	var cardOnly bool
	var maxPages int
	var userIDs []string
	var userEmail, userName string
	var filterMe bool
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Write a CSV of expenses missing memos (edit memo then apply)",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			c, err := clientFromFlags(*token, *baseURL)
			if err != nil {
				return err
			}
			uid, err := resolveExpenseUserIDs(ctx, c, userIDs, userEmail, userName, filterMe)
			if err != nil {
				return err
			}
			params := brex.ListExpensesParams{
				Limit:   100,
				Expand:  []string{"merchant", "user"},
				UserIDs: uid,
			}
			if cardOnly {
				params.ExpenseType = []string{"CARD"}
			}
			all, err := c.ListAllExpenses(ctx, params, maxPages)
			if err != nil {
				return err
			}
			missing := filterMissingMemo(all, cardOnly)
			f, err := os.Create(outPath)
			if err != nil {
				return err
			}
			defer f.Close()
			w := csv.NewWriter(f)
			header := []string{
				"expense_id", "brex_user_id", "user_display_name", "purchased_at", "amount_minor", "currency",
				"merchant_descriptor", "category", "dashboard_url", "suggested_memo", "memo",
			}
			if err := w.Write(header); err != nil {
				return err
			}
			for _, e := range missing {
				amt, cur := moneyFields(e)
				merchant := merchantDescriptor(e)
				suggested := draftMemoScaffold(e)
				brexUID := e.UserID
				if brexUID == "" && e.User != nil {
					brexUID = e.User.ID
				}
				row := []string{
					e.ID,
					brexUID,
					userDisplayName(e),
					e.PurchasedAt,
					strconv.FormatInt(amt, 10),
					cur,
					merchant,
					e.Category,
					e.DashboardURL,
					suggested,
					"", // user fills memo (or copy from suggested_memo)
				}
				if err := w.Write(row); err != nil {
					return err
				}
			}
			w.Flush()
			if err := w.Error(); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStderr(), "wrote %d row(s) to %s\n", len(missing), outPath)
			return nil
		},
	}
	cmd.Flags().StringVarP(&outPath, "output", "o", "brex-memos.csv", "output CSV path")
	cmd.Flags().BoolVar(&cardOnly, "card-only", true, "only CARD expenses")
	cmd.Flags().IntVar(&maxPages, "max-pages", 0, "max list pages (0 = all)")
	cmd.Flags().StringSliceVar(&userIDs, "user-id", nil, "only expenses for Brex user id(s); repeat or comma-separated")
	cmd.Flags().StringVar(&userEmail, "user-email", "", "resolve user id from email (needs users.readonly)")
	cmd.Flags().StringVar(&userName, "user-name", "", "resolve user id from full name (needs users.readonly)")
	cmd.Flags().BoolVar(&filterMe, "me", false, "only expenses for the authenticated user")
	return cmd
}

func applyCmd(token, baseURL *string) *cobra.Command {
	var csvPath string
	var dryRun bool
	var sleepMs int
	var useSuggested bool
	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Upload memos from a CSV (expects expense_id and memo, or --use-suggested)",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			c, err := clientFromFlags(*token, *baseURL)
			if err != nil {
				return err
			}
			f, err := os.Open(csvPath)
			if err != nil {
				return err
			}
			defer f.Close()
			r := csv.NewReader(f)
			r.ReuseRecord = true
			records, err := r.ReadAll()
			if err != nil {
				return err
			}
			if len(records) < 2 {
				return errors.New("csv must have a header row and at least one data row")
			}
			header := records[0]
			idx := map[string]int{}
			for i, h := range header {
				idx[strings.TrimSpace(strings.ToLower(h))] = i
			}
			idCol, ok := idx["expense_id"]
			if !ok {
				idCol, ok = idx["id"]
			}
			if !ok {
				return errors.New("csv must include expense_id column")
			}
			memoCol, ok := idx["memo"]
			if !ok && !useSuggested {
				return errors.New("csv must include memo column (or pass --use-suggested)")
			}
			suggestedCol := -1
			if useSuggested {
				suggestedCol, ok = idx["suggested_memo"]
				if !ok {
					return errors.New("csv must include suggested_memo when using --use-suggested")
				}
			}

			var applied, skipped int
			for _, row := range records[1:] {
				id := strings.TrimSpace(row[idCol])
				if id == "" {
					continue
				}
				var memo string
				if useSuggested {
					if suggestedCol < len(row) {
						memo = strings.TrimSpace(row[suggestedCol])
					}
				} else {
					if memoCol < len(row) {
						memo = strings.TrimSpace(row[memoCol])
					}
				}
				if memo == "" {
					skipped++
					continue
				}
				if dryRun {
					fmt.Fprintf(cmd.OutOrStdout(), "[dry-run] %s <- %q\n", id, memo)
					applied++
					continue
				}
				if err := c.UpdateExpenseMemo(ctx, id, memo); err != nil {
					return fmt.Errorf("update %s: %w", id, err)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "updated %s\n", id)
				applied++
				if sleepMs > 0 {
					select {
					case <-ctx.Done():
						return ctx.Err()
					case <-time.After(time.Duration(sleepMs) * time.Millisecond):
					}
				}
			}
			fmt.Fprintf(cmd.OutOrStderr(), "done: %d applied/printed, %d skipped (empty memo)\n", applied, skipped)
			return nil
		},
	}
	cmd.Flags().StringVar(&csvPath, "csv", "", "path to CSV from export")
	_ = cmd.MarkFlagRequired("csv")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print actions without calling PUT")
	cmd.Flags().IntVar(&sleepMs, "sleep-ms", 0, "wait between API writes (rate limiting)")
	cmd.Flags().BoolVar(&useSuggested, "use-suggested", false, "use suggested_memo column instead of memo")
	return cmd
}

// resolveExpenseUserIDs builds user_id[] query values for list expenses.
// Pass explicit ids from the dashboard (e.g. cuuser_…) or resolve via Team API.
func resolveExpenseUserIDs(ctx context.Context, c *brex.Client, userIDs []string, userEmail, userName string, useMe bool) ([]string, error) {
	n := 0
	if len(userIDs) > 0 {
		n++
	}
	if strings.TrimSpace(userEmail) != "" {
		n++
	}
	if strings.TrimSpace(userName) != "" {
		n++
	}
	if useMe {
		n++
	}
	if n > 1 {
		return nil, errors.New("use only one of --user-id, --user-email, --user-name, or --me")
	}

	switch {
	case useMe:
		u, err := c.GetMe(ctx)
		if err != nil {
			return nil, fmt.Errorf("get current user (needs users.readonly on token): %w", err)
		}
		return []string{u.ID}, nil
	case strings.TrimSpace(userEmail) != "":
		items, _, err := c.ListUsers(ctx, brex.ListUsersParams{
			Limit: 10,
			Email: strings.TrimSpace(userEmail),
		})
		if err != nil {
			return nil, fmt.Errorf("list users by email (needs users.readonly): %w", err)
		}
		if len(items) != 1 {
			return nil, fmt.Errorf("expected 1 user for email %q, got %d", strings.TrimSpace(userEmail), len(items))
		}
		return []string{items[0].ID}, nil
	case strings.TrimSpace(userName) != "":
		allUsers, err := c.ListAllUsers(ctx, 1000, 0)
		if err != nil {
			return nil, fmt.Errorf("list users (needs users.readonly): %w", err)
		}
		id, err := brex.FindUserIDByDisplayName(allUsers, userName)
		if err != nil {
			return nil, err
		}
		return []string{id}, nil
	case len(userIDs) > 0:
		out := flattenCommaIDs(userIDs)
		for i := range out {
			out[i] = strings.TrimSpace(out[i])
		}
		var nonEmpty []string
		for _, id := range out {
			if id != "" {
				nonEmpty = append(nonEmpty, id)
			}
		}
		if len(nonEmpty) == 0 {
			return nil, errors.New("no user ids passed to --user-id")
		}
		return nonEmpty, nil
	default:
		return nil, nil
	}
}

func flattenCommaIDs(ids []string) []string {
	var out []string
	for _, part := range ids {
		for _, s := range strings.Split(part, ",") {
			s = strings.TrimSpace(s)
			if s != "" {
				out = append(out, s)
			}
		}
	}
	return out
}

func filterMissingMemo(all []brex.Expense, cardOnly bool) []brex.Expense {
	var out []brex.Expense
	for _, e := range all {
		if strings.TrimSpace(e.Memo) != "" {
			continue
		}
		if cardOnly && e.ExpenseType != "" && e.ExpenseType != "CARD" {
			continue
		}
		out = append(out, e)
	}
	return out
}

func moneyFields(e brex.Expense) (int64, string) {
	if e.PurchasedAmount != nil {
		return e.PurchasedAmount.Amount, e.PurchasedAmount.Currency
	}
	if e.OriginalAmount != nil {
		return e.OriginalAmount.Amount, e.OriginalAmount.Currency
	}
	if e.BillingAmount != nil {
		return e.BillingAmount.Amount, e.BillingAmount.Currency
	}
	return 0, ""
}

func merchantDescriptor(e brex.Expense) string {
	if e.Merchant != nil {
		return e.Merchant.RawDescriptor
	}
	return ""
}

func userDisplayName(e brex.Expense) string {
	if e.User != nil {
		return strings.TrimSpace(e.User.FirstName + " " + e.User.LastName)
	}
	return ""
}

// draftMemoScaffold is a concrete starting sentence for the memo column (fill bracketed parts).
func draftMemoScaffold(e brex.Expense) string {
	who := userDisplayName(e)
	if who == "" {
		who = "I"
	}
	vendor := merchantDescriptor(e)
	if vendor == "" {
		vendor = "this vendor"
	}
	cat := strings.ReplaceAll(e.Category, "_", " ")
	if cat == "" {
		cat = "card"
	}
	cat = strings.ToLower(cat)
	return fmt.Sprintf("%s — %s (%s): payment for [product/plan]; for [business purpose].",
		who, vendor, cat)
}

func formatExpenseLine(e brex.Expense) string {
	amt, cur := moneyFields(e)
	m := merchantDescriptor(e)
	// Amount is in the smallest currency unit (e.g. cents for USD, whole units for JPY).
	return fmt.Sprintf("%s\t%d %s\t%s\t%s", e.ID, amt, cur, m, e.DashboardURL)
}
