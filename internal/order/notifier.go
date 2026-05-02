package order

import "context"

// MultiNotifier dispatches each event to every child Notifier, in order.
// A panic in one child is recovered and does not prevent later children
// from being called. Each child is itself responsible for non-blocking
// delivery (per the Notifier contract); MultiNotifier.Notify therefore
// returns quickly even with several children.
type MultiNotifier struct {
	children []Notifier
}

// NewMultiNotifier returns a MultiNotifier wrapping the given children.
// Pass zero children to get a no-op notifier.
func NewMultiNotifier(children ...Notifier) *MultiNotifier {
	return &MultiNotifier{children: children}
}

// Notify fans evt out to every child. Panics in any child are recovered
// silently — the child is expected to log internally if it cares.
func (m *MultiNotifier) Notify(ctx context.Context, evt NotificationEvent) {
	for _, c := range m.children {
		func(c Notifier) {
			defer func() { _ = recover() }()
			c.Notify(ctx, evt)
		}(c)
	}
}
