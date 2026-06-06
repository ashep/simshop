package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// activeStatuses are the non-terminal order statuses shown by `order list` when
// no --status filter is given. Terminal statuses (delivered, cancelled, returned,
// refunded) are excluded; pass `--status all` to include them.
var activeStatuses = []string{
	"new",
	"awaiting_payment",
	"payment_processing",
	"payment_hold",
	"paid",
	"processing",
	"shipped",
	"refund_requested",
}

// resolveStatusFilter maps the --status flag to the statuses sent to GET /orders:
// empty defaults to the active set, "all" (anywhere in the list) clears the filter,
// and any other value list passes through unchanged.
func resolveStatusFilter(flag []string) []string {
	if len(flag) == 0 {
		return activeStatuses
	}
	for _, s := range flag {
		if s == "all" {
			return nil
		}
	}
	return flag
}

// operatorStatuses are the statuses an operator may set via the API.
var operatorStatuses = map[string]bool{
	"processing":       true,
	"shipped":          true,
	"delivered":        true,
	"cancelled":        true,
	"refund_requested": true,
	"returned":         true,
	"refunded":         true,
}

func newOrderCmd(o *globalOpts) *cobra.Command {
	cmd := &cobra.Command{Use: "order", Short: "Manage orders"}
	cmd.AddCommand(newOrderListCmd(o), newOrderGetCmd(o), newOrderSetStatusCmd(o))
	return cmd
}

func newOrderListCmd(o *globalOpts) *cobra.Command {
	var status []string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List orders",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cl, err := o.client()
			if err != nil {
				return err
			}
			orders, err := cl.ListOrders(cmd.Context(), resolveStatusFilter(status))
			if err != nil {
				return err
			}
			if *o.jsonOut {
				return RenderJSON(cmd.OutOrStdout(), orders)
			}
			return RenderOrders(cmd.OutOrStdout(), orders)
		},
	}
	cmd.Flags().StringSliceVar(&status, "status", nil, "filter by status (comma-separated)")
	return cmd
}

func newOrderGetCmd(o *globalOpts) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Show a single order",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := o.client()
			if err != nil {
				return err
			}
			order, err := cl.GetOrder(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if *o.jsonOut {
				return RenderJSON(cmd.OutOrStdout(), order)
			}
			return RenderOrderDetail(cmd.OutOrStdout(), order)
		},
	}
}

// setStatusResult is one order's outcome from `order set-status`.
type setStatusResult struct {
	ID     string `json:"id"`
	Status string `json:"status,omitempty"`
	Error  string `json:"error,omitempty"`
}

func newOrderSetStatusCmd(o *globalOpts) *cobra.Command {
	var tracking, note string
	cmd := &cobra.Command{
		Use:   "set-status <status> <id> [<id>...]",
		Short: "Change the status of one or more orders",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			status, ids := args[0], args[1:]
			if !operatorStatuses[status] {
				return fmt.Errorf("invalid status %q; allowed: %s", status, allowedStatusList())
			}
			cl, err := o.client()
			if err != nil {
				return err
			}

			results := make([]setStatusResult, 0, len(ids))
			failed := 0
			for _, raw := range ids {
				id, err := cl.ResolveOrderID(cmd.Context(), raw)
				if err == nil {
					var newStatus string
					newStatus, err = cl.SetStatus(cmd.Context(), id, status, tracking, note)
					if err == nil {
						results = append(results, setStatusResult{ID: id, Status: newStatus})
						if !*o.jsonOut {
							_, _ = fmt.Fprintf(cmd.OutOrStdout(), "order %s status: %s\n", id, newStatus)
						}
						continue
					}
				} else {
					id = raw // resolution failed; report the id the user typed
				}
				failed++
				results = append(results, setStatusResult{ID: id, Error: err.Error()})
				if !*o.jsonOut {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "order %s failed: %v\n", id, err)
				}
			}

			if *o.jsonOut {
				if err := RenderJSON(cmd.OutOrStdout(), results); err != nil {
					return err
				}
			}
			if failed > 0 {
				return fmt.Errorf("%d of %d orders failed", failed, len(ids))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&tracking, "tracking", "", "tracking number (required by the server when status=shipped)")
	cmd.Flags().StringVar(&note, "note", "", "optional note recorded in order history")
	return cmd
}

func allowedStatusList() string {
	out := make([]string, 0, len(operatorStatuses))
	for s := range operatorStatuses {
		out = append(out, s)
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}
