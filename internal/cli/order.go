package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

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
			orders, err := cl.ListOrders(cmd.Context(), status)
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

func newOrderSetStatusCmd(o *globalOpts) *cobra.Command {
	var tracking, note string
	cmd := &cobra.Command{
		Use:   "set-status <id> <status>",
		Short: "Change an order's status",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, status := args[0], args[1]
			if !operatorStatuses[status] {
				return fmt.Errorf("invalid status %q; allowed: %s", status, allowedStatusList())
			}
			cl, err := o.client()
			if err != nil {
				return err
			}
			newStatus, err := cl.SetStatus(cmd.Context(), id, status, tracking, note)
			if err != nil {
				return err
			}
			if *o.jsonOut {
				return RenderJSON(cmd.OutOrStdout(), map[string]string{"id": id, "status": newStatus})
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "order %s status: %s\n", id, newStatus)
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
