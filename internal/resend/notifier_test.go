package resend

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ashep/simshop/internal/order"
)

type fakeSender struct {
	mu      sync.Mutex
	calls   []Email
	err     error
	respond func(int) error
	callCnt atomic.Int32
}

func (f *fakeSender) SendEmail(_ context.Context, e Email) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, e)
	n := int(f.callCnt.Add(1))
	if f.respond != nil {
		return f.respond(n)
	}
	return f.err
}

func (f *fakeSender) snapshot() []Email {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Email, len(f.calls))
	copy(out, f.calls)
	return out
}

type fakeReader struct {
	rec *order.Record
	err error
}

func (f *fakeReader) List(context.Context) ([]order.Record, error) { return nil, nil }
func (f *fakeReader) GetByID(_ context.Context, _ string) (*order.Record, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.rec, nil
}
func (f *fakeReader) GetStatus(context.Context, string) (string, error) { return "", nil }

type fakeProducts struct{ titles map[string]string }

func (f *fakeProducts) Title(id string) string { return f.titles[id] }

type fakeShop struct{ names map[string]string }

func (f *fakeShop) Name(lang string) string { return f.names[lang] }

type fakeTemplates struct {
	subject  string
	html     string
	text     string
	err      error
	lastData TemplateData
	mu       sync.Mutex
}

func (f *fakeTemplates) Render(_, _ string, data TemplateData) (string, string, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastData = data
	if f.err != nil {
		return "", "", "", f.err
	}
	return f.subject, f.html, f.text, nil
}

func newTestNotifier(t *testing.T, send *fakeSender, rd *fakeReader, tpl *fakeTemplates) *Notifier {
	t.Helper()
	n := NewNotifier(
		send, "from@example.com", "https://shop.example/order?id={id}",
		rd,
		&fakeProducts{titles: map[string]string{"widget": "Widget"}},
		&fakeShop{names: map[string]string{"en": "My Shop", "uk": "Мій магазин"}},
		tpl,
		zerolog.Nop(),
	)
	n.sleepFn = func(time.Duration) {}
	return n
}

func sampleRecord() *order.Record {
	return &order.Record{
		ID:        "018f4e3a-0000-7000-8000-000000000099",
		ProductID: "widget", Status: "paid", Email: "buyer@example.com",
		Price: 4900, Currency: "USD", Lang: "en",
		FirstName: "Jane", LastName: "Doe",
		Country: "us", City: "NYC", Phone: "+1", Address: "1 St",
		Attrs: []order.Attr{}, History: []order.HistoryEntry{}, Invoices: []order.Invoice{},
	}
}

func TestNotifier(main *testing.T) {
	main.Run("DispatchesPaid", func(t *testing.T) {
		send := &fakeSender{}
		rd := &fakeReader{rec: sampleRecord()}
		tpl := &fakeTemplates{subject: "Paid 0123", html: "<p>Hi</p>", text: "Hi"}
		n := newTestNotifier(t, send, rd, tpl)
		n.Start()
		defer n.Stop()
		n.Notify(context.Background(), order.NotificationEvent{OrderID: "018f4e3a-0000-7000-8000-000000000099", Status: "paid"})

		require.Eventually(t, func() bool { return len(send.snapshot()) == 1 }, time.Second, 10*time.Millisecond)
		e := send.snapshot()[0]
		assert.Equal(t, "from@example.com", e.From)
		assert.Equal(t, "buyer@example.com", e.To)
		assert.Equal(t, "Paid 0123", e.Subject)
		assert.Equal(t, "<p>Hi</p>", e.HTML)
		assert.Equal(t, "Hi", e.Text)

		// TemplateData population
		d := tpl.lastData
		assert.Equal(t, "018f4e3a-0000-7000-8000-000000000099", d.OrderID)
		assert.Equal(t, "018f4e3a-0000", d.OrderShortID)
		assert.Equal(t, "Jane Doe", d.CustomerName)
		assert.Equal(t, "Widget", d.ProductTitle)
		assert.Equal(t, "49.00 USD", d.Total)
		assert.Equal(t, "My Shop", d.ShopName)
		assert.Equal(t, "https://shop.example/order?id=018f4e3a-0000-7000-8000-000000000099", d.OrderURL)
	})

	for _, st := range []string{"new", "awaiting_payment", "payment_processing", "payment_hold", "cancelled", "processing", "returned"} {
		main.Run("Skips_"+st, func(t *testing.T) {
			send := &fakeSender{}
			rd := &fakeReader{rec: sampleRecord()}
			tpl := &fakeTemplates{}
			n := newTestNotifier(t, send, rd, tpl)
			n.Start()
			defer n.Stop()
			n.Notify(context.Background(), order.NotificationEvent{OrderID: "x", Status: st})
			time.Sleep(50 * time.Millisecond)
			assert.Empty(t, send.snapshot(), "must not send email for status %q", st)
		})
	}

	main.Run("DispatchesShippedDeliveredRefundRequestedRefunded", func(t *testing.T) {
		for _, st := range []string{"shipped", "delivered", "refund_requested", "refunded"} {
			t.Run(st, func(t *testing.T) {
				send := &fakeSender{}
				rd := &fakeReader{rec: sampleRecord()}
				tpl := &fakeTemplates{subject: "subj", html: "<p/>", text: "x"}
				n := newTestNotifier(t, send, rd, tpl)
				n.Start()
				defer n.Stop()
				n.Notify(context.Background(), order.NotificationEvent{OrderID: "x", Status: st})
				require.Eventually(t, func() bool { return len(send.snapshot()) == 1 }, time.Second, 10*time.Millisecond)
			})
		}
	})

	main.Run("ReaderErrorDropsEvent", func(t *testing.T) {
		send := &fakeSender{}
		rd := &fakeReader{err: errors.New("db down")}
		tpl := &fakeTemplates{}
		n := newTestNotifier(t, send, rd, tpl)
		n.Start()
		defer n.Stop()
		n.Notify(context.Background(), order.NotificationEvent{OrderID: "x", Status: "paid"})
		time.Sleep(50 * time.Millisecond)
		assert.Empty(t, send.snapshot())
	})

	main.Run("OrderNotFoundDropsEvent", func(t *testing.T) {
		send := &fakeSender{}
		rd := &fakeReader{err: order.ErrNotFound}
		tpl := &fakeTemplates{}
		n := newTestNotifier(t, send, rd, tpl)
		n.Start()
		defer n.Stop()
		n.Notify(context.Background(), order.NotificationEvent{OrderID: "x", Status: "paid"})
		time.Sleep(50 * time.Millisecond)
		assert.Empty(t, send.snapshot())
	})

	main.Run("TemplateErrorDropsEvent", func(t *testing.T) {
		send := &fakeSender{}
		rd := &fakeReader{rec: sampleRecord()}
		tpl := &fakeTemplates{err: errors.New("missing var")}
		n := newTestNotifier(t, send, rd, tpl)
		n.Start()
		defer n.Stop()
		n.Notify(context.Background(), order.NotificationEvent{OrderID: "x", Status: "paid"})
		time.Sleep(50 * time.Millisecond)
		assert.Empty(t, send.snapshot())
	})

	main.Run("RetriesOn5xxThenSucceeds", func(t *testing.T) {
		send := &fakeSender{respond: func(n int) error {
			if n < 3 {
				return &APIError{HTTPStatus: http.StatusInternalServerError, Message: "boom"}
			}
			return nil
		}}
		rd := &fakeReader{rec: sampleRecord()}
		tpl := &fakeTemplates{subject: "s", html: "h", text: "t"}
		n := newTestNotifier(t, send, rd, tpl)
		n.Start()
		defer n.Stop()
		n.Notify(context.Background(), order.NotificationEvent{OrderID: "x", Status: "paid"})
		require.Eventually(t, func() bool { return int(send.callCnt.Load()) == 3 }, time.Second, 10*time.Millisecond)
	})

	main.Run("DropsOn4xxNoRetry", func(t *testing.T) {
		send := &fakeSender{err: &APIError{HTTPStatus: http.StatusUnprocessableEntity, Message: "bad email"}}
		rd := &fakeReader{rec: sampleRecord()}
		tpl := &fakeTemplates{subject: "s", html: "h", text: "t"}
		n := newTestNotifier(t, send, rd, tpl)
		n.Start()
		defer n.Stop()
		n.Notify(context.Background(), order.NotificationEvent{OrderID: "x", Status: "paid"})
		time.Sleep(50 * time.Millisecond)
		assert.EqualValues(t, 1, send.callCnt.Load())
	})

	main.Run("HonorsRetryAfterOn429", func(t *testing.T) {
		sleepCh := make(chan time.Duration, 4)
		send := &fakeSender{respond: func(n int) error {
			if n < 2 {
				return &APIError{HTTPStatus: http.StatusTooManyRequests, RetryAfter: 7 * time.Second}
			}
			return nil
		}}
		rd := &fakeReader{rec: sampleRecord()}
		tpl := &fakeTemplates{subject: "s", html: "h", text: "t"}
		n := NewNotifier(send, "from@example.com", "https://shop.example/order?id={id}", rd,
			&fakeProducts{}, &fakeShop{}, tpl, zerolog.Nop())
		n.sleepFn = func(d time.Duration) { sleepCh <- d }
		n.Start()
		defer n.Stop()
		n.Notify(context.Background(), order.NotificationEvent{OrderID: "x", Status: "paid"})
		require.Eventually(t, func() bool { return int(send.callCnt.Load()) == 2 }, time.Second, 10*time.Millisecond)
		select {
		case d := <-sleepCh:
			assert.Equal(t, 7*time.Second, d)
		case <-time.After(time.Second):
			t.Fatal("expected sleepFn to be called")
		}
	})

	main.Run("StopDrainsBufferedEvents", func(t *testing.T) {
		send := &fakeSender{}
		rd := &fakeReader{rec: sampleRecord()}
		tpl := &fakeTemplates{subject: "s", html: "h", text: "t"}
		n := newTestNotifier(t, send, rd, tpl)
		n.Start()
		for i := 0; i < 5; i++ {
			n.Notify(context.Background(), order.NotificationEvent{OrderID: "x", Status: "paid"})
		}
		n.Stop()
		assert.Equal(t, 5, len(send.snapshot()))
	})

	main.Run("BufferFullDropsSilently", func(t *testing.T) {
		send := &fakeSender{respond: func(int) error {
			time.Sleep(time.Second) // hold the worker so the channel fills
			return nil
		}}
		rd := &fakeReader{rec: sampleRecord()}
		tpl := &fakeTemplates{subject: "s", html: "h", text: "t"}
		n := newTestNotifier(t, send, rd, tpl)
		n.Start()
		defer n.Stop()
		// Fill: bufferSize + 1 events; the +1 must not block.
		for i := 0; i < bufferSize+10; i++ {
			n.Notify(context.Background(), order.NotificationEvent{OrderID: "x", Status: "paid"})
		}
		// No assertion beyond "did not block"; reaching this line is the test.
		_ = strings.Builder{}
	})
}
