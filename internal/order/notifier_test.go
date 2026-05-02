package order

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

type recordingNotifier struct {
	events []NotificationEvent
}

func (n *recordingNotifier) Notify(_ context.Context, evt NotificationEvent) {
	n.events = append(n.events, evt)
}

type panickingNotifier struct{}

func (p *panickingNotifier) Notify(_ context.Context, _ NotificationEvent) {
	panic("boom")
}

func TestMultiNotifier(main *testing.T) {
	main.Run("FanoutInvokesEachChildOnce", func(t *testing.T) {
		a := &recordingNotifier{}
		b := &recordingNotifier{}
		m := NewMultiNotifier(a, b)
		evt := NotificationEvent{OrderID: "o1", Status: "paid"}
		m.Notify(context.Background(), evt)
		assert.Equal(t, []NotificationEvent{evt}, a.events)
		assert.Equal(t, []NotificationEvent{evt}, b.events)
	})

	main.Run("PanicInOneChildDoesNotSkipNext", func(t *testing.T) {
		p := &panickingNotifier{}
		c := &recordingNotifier{}
		m := NewMultiNotifier(p, c)
		evt := NotificationEvent{OrderID: "o2", Status: "shipped"}
		// Must not panic.
		m.Notify(context.Background(), evt)
		assert.Equal(t, []NotificationEvent{evt}, c.events)
	})

	main.Run("ZeroChildrenIsNoOp", func(t *testing.T) {
		m := NewMultiNotifier()
		m.Notify(context.Background(), NotificationEvent{OrderID: "o3", Status: "paid"})
		// No assertion — just must not panic.
	})
}
