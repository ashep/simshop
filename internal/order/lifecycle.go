package order

// fulfillmentOrderStatuses are the operator-owned states. Once an order has
// entered fulfillment, invoice webhook events become informational and must
// not move the order back into a payment state — see
// ShouldApplyInvoiceTransition for the full rule.
var fulfillmentOrderStatuses = map[string]bool{
	"processing":       true,
	"shipped":          true,
	"delivered":        true,
	"refund_requested": true,
	"returned":         true,
}

// ShouldApplyInvoiceTransition reports whether candidate (the order_status
// derived from the latest invoice event for an order) should override the
// current order_status. The rule:
//
//   - Idempotent: equal current and candidate → no-op.
//   - candidate == "refunded" always wins (customer money was returned), as
//     long as the order is not already refunded.
//   - Once an order has entered fulfillment, invoice events are informational
//     (the operator owns the lifecycle from then on). Exception: refunded,
//     handled above.
//   - "paid" is stable against payment_* and cancelled — only "refunded"
//     moves it forward.
//   - Within the pre-paid cluster {new, awaiting_payment, payment_processing,
//     payment_hold, cancelled}, invoice events freely drive the order. This
//     is what makes "retry after failure" work: cancelled → payment_processing
//     → paid is permitted.
func ShouldApplyInvoiceTransition(current, candidate string) bool {
	if current == candidate {
		return false
	}
	if candidate == "refunded" {
		return current != "refunded"
	}
	if fulfillmentOrderStatuses[current] || current == "refunded" {
		return false
	}
	if current == "paid" {
		return false
	}
	return true
}

// allowedOperatorTransitions enumerates the forward-only transitions an operator
// may drive via PATCH /orders/{id}/status. Equal current and target is rejected
// here (returns false) so the writer's same check is trivially correct; the
// handler converts the equal case to an idempotent 200 before reaching the
// service. Targets outside this map are operator-forbidden:
//   - The pre-paid cluster (new, awaiting_payment, payment_processing,
//     payment_hold, cancelled) is webhook-owned.
//   - "refunded" is terminal.
var allowedOperatorTransitions = map[string]map[string]bool{
	"paid":             {"processing": true, "refund_requested": true, "refunded": true},
	"processing":       {"shipped": true, "refund_requested": true, "refunded": true},
	"shipped":          {"delivered": true, "refund_requested": true},
	"delivered":        {"refund_requested": true},
	"refund_requested": {"processing": true, "shipped": true, "delivered": true, "returned": true, "refunded": true},
	"returned":         {"refunded": true},
}

// ShouldApplyOperatorTransition reports whether an operator may drive an order
// from current to target. See allowedOperatorTransitions for the rule. Equal
// values return false (not a transition). The function is the sole authority
// on operator-driven transitions; the handler calls it pre-lock and the writer
// calls it again under SELECT … FOR UPDATE.
func ShouldApplyOperatorTransition(current, target string) bool {
	if current == target {
		return false
	}
	targets, ok := allowedOperatorTransitions[current]
	if !ok {
		return false
	}
	return targets[target]
}

// InvoiceStatusToOrderStatus maps a persisted invoice_status (the latest event
// in the invoice timeline) to the order_status it implies. ok=false means the
// invoice status does not drive the order lifecycle.
func InvoiceStatusToOrderStatus(s string) (status string, ok bool) {
	switch s {
	case InvoiceStatusProcessing:
		return "payment_processing", true
	case InvoiceStatusHold:
		return "payment_hold", true
	case InvoiceStatusPaid:
		return "paid", true
	case InvoiceStatusFailed:
		return "cancelled", true
	case InvoiceStatusReversed:
		return "refunded", true
	}
	return "", false
}
