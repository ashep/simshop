package order

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInvoiceStatusToOrderStatus(main *testing.T) {
	cases := []struct {
		in  string
		out string
		ok  bool
	}{
		{InvoiceStatusProcessing, "payment_processing", true},
		{InvoiceStatusHold, "payment_hold", true},
		{InvoiceStatusPaid, "paid", true},
		{InvoiceStatusFailed, "cancelled", true},
		{InvoiceStatusReversed, "refunded", true},
		{"unknown", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		main.Run(c.in, func(t *testing.T) {
			got, ok := InvoiceStatusToOrderStatus(c.in)
			assert.Equal(t, c.out, got)
			assert.Equal(t, c.ok, ok)
		})
	}
}

func TestShouldApplyInvoiceTransition(main *testing.T) {
	cases := []struct {
		current   string
		candidate string
		want      bool
	}{
		// Pre-paid cluster: invoice events drive the lifecycle freely.
		{"awaiting_payment", "payment_processing", true},
		{"awaiting_payment", "paid", true},
		{"awaiting_payment", "cancelled", true},
		{"payment_processing", "payment_hold", true},
		{"payment_processing", "paid", true},
		{"payment_hold", "paid", true},
		{"payment_processing", "cancelled", true},

		// Retry after failure: cancelled is re-enterable.
		{"cancelled", "payment_processing", true},
		{"cancelled", "payment_hold", true},
		{"cancelled", "paid", true},
		{"cancelled", "cancelled", false}, // idempotent

		// Idempotent.
		{"paid", "paid", false},
		{"awaiting_payment", "awaiting_payment", false},

		// paid is stable against payment_* and cancelled — only refunded moves it forward.
		{"paid", "payment_processing", false},
		{"paid", "cancelled", false},
		{"paid", "refunded", true},

		// Fulfillment is operator-owned: invoice events do not downgrade or move it
		// (except refunded, which always wins).
		{"processing", "cancelled", false},
		{"processing", "paid", false},
		{"processing", "payment_processing", false},
		{"processing", "refunded", true},
		{"shipped", "cancelled", false},
		{"shipped", "refunded", true},
		{"delivered", "refunded", true},

		// refunded is terminal.
		{"refunded", "paid", false},
		{"refunded", "refunded", false},
		{"refunded", "payment_processing", false},
	}
	for _, c := range cases {
		main.Run(c.current+"_"+c.candidate, func(t *testing.T) {
			assert.Equal(t, c.want, ShouldApplyInvoiceTransition(c.current, c.candidate))
		})
	}
}

func TestShouldApplyOperatorTransition(main *testing.T) {
	cases := []struct {
		name    string
		current string
		target  string
		want    bool
	}{
		// Allowed transitions.
		{"PaidToProcessing", "paid", "processing", true},
		{"PaidToRefunded", "paid", "refunded", true},
		{"ProcessingToShipped", "processing", "shipped", true},
		{"ProcessingToRefunded", "processing", "refunded", true},
		{"ShippedToDelivered", "shipped", "delivered", true},
		{"ShippedToRefundRequested", "shipped", "refund_requested", true},
		{"DeliveredToRefundRequested", "delivered", "refund_requested", true},
		{"RefundRequestedToReturned", "refund_requested", "returned", true},
		{"RefundRequestedToRefunded", "refund_requested", "refunded", true},
		{"ReturnedToRefunded", "returned", "refunded", true},

		// Equal current and target — idempotent (handler-level concern).
		{"EqualPaid", "paid", "paid", false},
		{"EqualShipped", "shipped", "shipped", false},
		{"EqualRefunded", "refunded", "refunded", false},

		// Operator must not drive the pre-paid cluster.
		{"NewToProcessing", "new", "processing", false},
		{"AwaitingPaymentToProcessing", "awaiting_payment", "processing", false},
		{"PaymentProcessingToProcessing", "payment_processing", "processing", false},
		{"PaymentHoldToProcessing", "payment_hold", "processing", false},
		{"CancelledToProcessing", "cancelled", "processing", false},

		// Operator cannot skip steps.
		{"PaidToShipped", "paid", "shipped", false},
		{"PaidToDelivered", "paid", "delivered", false},
		{"ProcessingToDelivered", "processing", "delivered", false},

		// Operator cannot move backwards.
		{"DeliveredToShipped", "delivered", "shipped", false},
		{"ShippedToProcessing", "shipped", "processing", false},
		{"ProcessingToPaid", "processing", "paid", false},

		// Refunded is terminal.
		{"RefundedToAnything", "refunded", "processing", false},
		{"RefundedToShipped", "refunded", "shipped", false},

		// Operator never sets payment-cluster targets.
		{"PaidToCancelled", "paid", "cancelled", false},
		{"ProcessingToCancelled", "processing", "cancelled", false},
		{"ShippedToCancelled", "shipped", "cancelled", false},
	}
	for _, c := range cases {
		main.Run(c.name, func(t *testing.T) {
			got := ShouldApplyOperatorTransition(c.current, c.target)
			assert.Equal(t, c.want, got, "current=%q target=%q", c.current, c.target)
		})
	}
}
