package main

import (
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
  brex-memo export --output memos.csv
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
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Print expenses with an empty memo",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			c, err := clientFromFlags(*token, *baseURL)
			if err != nil {
				return err
			}
			params := brex.ListExpensesParams{
				Limit:  100,
				Expand: []string{"merchant", "user"},
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
	return cmd
}

func exportCmd(token, baseURL *string) *cobra.Command {
	var outPath string
	var cardOnly bool
	var maxPages int
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Write a CSV of expenses missing memos (edit memo then apply)",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			c, err := clientFromFlags(*token, *baseURL)
			if err != nil {
				return err
			}
			params := brex.ListExpensesParams{
				Limit:  100,
				Expand: []string{"merchant", "user"},
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
				"expense_id", "purchased_at", "amount_minor", "currency", "merchant_descriptor",
				"category", "dashboard_url", "suggested_memo", "memo",
			}
			if err := w.Write(header); err != nil {
				return err
			}
			for _, e := range missing {
				amt, cur := moneyFields(e)
				merchant := merchantDescriptor(e)
				suggested := defaultSuggestedMemo(e)
				row := []string{
					e.ID,
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

func defaultSuggestedMemo(e brex.Expense) string {
	m := merchantDescriptor(e)
	if m != "" {
		return fmt.Sprintf("Business expense — %s", m)
	}
	if e.Category != "" {
		return fmt.Sprintf("Business expense — %s", strings.ReplaceAll(e.Category, "_", " "))
	}
	return "Business expense"
}

func formatExpenseLine(e brex.Expense) string {
	amt, cur := moneyFields(e)
	m := merchantDescriptor(e)
	// Amount is in the smallest currency unit (e.g. cents for USD, whole units for JPY).
	return fmt.Sprintf("%s\t%d %s\t%s\t%s", e.ID, amt, cur, m, e.DashboardURL)
}
