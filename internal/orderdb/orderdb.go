// Package orderdb persists orders to PostgreSQL.
package orderdb

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ashep/simshop/internal/order"
)

// Writer inserts an order and its attributes into PostgreSQL inside a single
// transaction so partial failure cannot leave an order without its attrs (or
// vice versa). Implements order.Writer and order.InvoiceWriter.
type Writer struct {
	db *pgxpool.Pool
}

// New returns a Writer backed by db.
func New(db *pgxpool.Pool) *Writer {
	return &Writer{db: db}
}

const insertOrderSQL = `INSERT INTO orders (
	product_id, email, price, currency, lang,
	first_name, middle_name, last_name,
	country, city, phone, address,
	customer_note
) VALUES (
	$1, $2, $3, $4, $5,
	$6, $7, $8,
	$9, $10, $11, $12,
	$13
) RETURNING id`

const insertAttrSQL = `INSERT INTO order_attrs
	(order_id, attr_name, attr_value, attr_price)
	VALUES ($1, $2, $3, $4)`

const insertHistorySQL = `INSERT INTO order_history (order_id, status) VALUES ($1, 'new')`

const insertInvoiceSQL = `INSERT INTO order_invoices
	(order_id, provider, id, page_url, amount, currency)
	VALUES ($1, $2, $3, $4, $5, $6)`

const updateOrderStatusAwaitingPaymentSQL = `UPDATE orders
	SET status = 'awaiting_payment', updated_at = CURRENT_TIMESTAMP
	WHERE id = $1`

const insertHistoryAwaitingPaymentSQL = `INSERT INTO order_history
	(order_id, status) VALUES ($1, 'awaiting_payment')`

// Write inserts a row for o into orders and one row per attr into order_attrs,
// then inserts the initial 'new' status into order_history. Returns the
// assigned order id. id, status, and timestamps are populated by database
// defaults.
func (w *Writer) Write(ctx context.Context, o order.Order) (string, error) {
	tx, err := w.db.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		// Rollback after Commit is a no-op (returns ErrTxClosed); ignore it.
		if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
			// Best-effort: surface unexpected rollback errors via the existing returned error.
			_ = rbErr
		}
	}()

	var orderID string
	if err := tx.QueryRow(ctx, insertOrderSQL,
		o.ProductID, o.Email, o.Price, o.Currency, o.Lang,
		o.FirstName, nullIfEmpty(o.MiddleName), o.LastName,
		o.Country, o.City, o.Phone, o.Address,
		nullIfEmpty(o.CustomerNote),
	).Scan(&orderID); err != nil {
		return "", fmt.Errorf("insert order: %w", err)
	}

	for _, a := range o.Attrs {
		if _, err := tx.Exec(ctx, insertAttrSQL, orderID, a.Name, a.Value, a.Price); err != nil {
			return "", fmt.Errorf("insert order_attr %q: %w", a.Name, err)
		}
	}

	if _, err := tx.Exec(ctx, insertHistorySQL, orderID); err != nil {
		return "", fmt.Errorf("insert order_history: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit tx: %w", err)
	}
	return orderID, nil
}

// AttachInvoice records an issued payment invoice and transitions the order
// to status='awaiting_payment' in a single transaction. Order, attrs, and the
// initial 'new' history row must already exist (created by Write).
func (w *Writer) AttachInvoice(ctx context.Context, orderID string, inv order.Invoice) error {
	tx, err := w.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		// Rollback after Commit is a no-op (returns ErrTxClosed); ignore it.
		if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
			// Best-effort: surface unexpected rollback errors via the existing returned error.
			_ = rbErr
		}
	}()

	if _, err := tx.Exec(ctx, insertInvoiceSQL,
		orderID, inv.Provider, inv.ID, inv.PageURL, inv.Amount, inv.Currency,
	); err != nil {
		return fmt.Errorf("insert order_invoice: %w", err)
	}

	tag, err := tx.Exec(ctx, updateOrderStatusAwaitingPaymentSQL, orderID)
	if err != nil {
		return fmt.Errorf("update order status: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("update order status: expected 1 row affected, got %d", tag.RowsAffected())
	}

	if _, err := tx.Exec(ctx, insertHistoryAwaitingPaymentSQL, orderID); err != nil {
		return fmt.Errorf("insert order_history: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// Reader reads orders and their attrs and history from PostgreSQL.
// Implements order.Reader.
type Reader struct {
	db *pgxpool.Pool
}

// NewReader returns a Reader backed by db.
func NewReader(db *pgxpool.Pool) *Reader {
	return &Reader{db: db}
}

const listOrdersSQL = `SELECT id::text, product_id, status::text, email, price, currency, lang,
	first_name, middle_name, last_name,
	country, city, phone, address,
	admin_note, customer_note,
	created_at, updated_at
FROM orders
ORDER BY created_at DESC`

const listAttrsSQL = `SELECT order_id::text, attr_name, attr_value, attr_price
FROM order_attrs
WHERE order_id = ANY($1::uuid[])`

const listHistorySQL = `SELECT id::text, order_id::text, status::text, note, created_at
FROM order_history
WHERE order_id = ANY($1::uuid[])
ORDER BY created_at ASC`

const listInvoicesSQL = `SELECT order_id::text, provider, id, page_url, amount, currency
	FROM order_invoices
	WHERE order_id = ANY($1::uuid[])
	ORDER BY created_at ASC`

const getOrderStatusSQL = `SELECT status::text FROM orders WHERE id = $1::uuid`

const updateOrderStatusSQL = `UPDATE orders
	SET status = $2::order_status, updated_at = CURRENT_TIMESTAMP
	WHERE id = $1::uuid`

const insertOrderHistoryNoteSQL = `INSERT INTO order_history
	(order_id, status, note) VALUES ($1::uuid, $2::order_status, $3)`

const lockOrderStatusSQL = `SELECT status::text FROM orders WHERE id = $1::uuid FOR UPDATE`

const insertInvoiceHistorySQL = `INSERT INTO invoice_history
	(order_id, invoice_id, provider, status, note, payload, event_at)
	VALUES ($1::uuid, $2, $3, $4::invoice_status, $5, $6, $7)
	ON CONFLICT (invoice_id, provider, status, event_at) DO NOTHING`

const latestInvoiceEventSQL = `SELECT status::text, note FROM invoice_history
	WHERE order_id = $1::uuid
	ORDER BY event_at DESC, created_at DESC
	LIMIT 1`

// List returns all orders, newest first, each populated with its attrs and
// history. Always returns a non-nil slice on success (possibly empty).
func (r *Reader) List(ctx context.Context) ([]order.Record, error) {
	rows, err := r.db.Query(ctx, listOrdersSQL)
	if err != nil {
		return nil, fmt.Errorf("query orders: %w", err)
	}

	out := []order.Record{}
	var ids []string
	for rows.Next() {
		var rec order.Record
		var middleName, adminNote, customerNote pgtype.Text
		if err := rows.Scan(
			&rec.ID, &rec.ProductID, &rec.Status, &rec.Email, &rec.Price, &rec.Currency, &rec.Lang,
			&rec.FirstName, &middleName, &rec.LastName,
			&rec.Country, &rec.City, &rec.Phone, &rec.Address,
			&adminNote, &customerNote,
			&rec.CreatedAt, &rec.UpdatedAt,
		); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan order: %w", err)
		}
		if middleName.Valid {
			v := middleName.String
			rec.MiddleName = &v
		}
		if adminNote.Valid {
			v := adminNote.String
			rec.AdminNote = &v
		}
		if customerNote.Valid {
			v := customerNote.String
			rec.CustomerNote = &v
		}
		rec.Attrs = []order.Attr{}
		rec.History = []order.HistoryEntry{}
		rec.Invoices = []order.Invoice{}
		out = append(out, rec)
		ids = append(ids, rec.ID)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iter orders: %w", err)
	}

	if len(out) == 0 {
		return out, nil
	}

	// Build the index after the slice is fully grown so &out[i] stays valid.
	indexByID := make(map[string]*order.Record, len(out))
	for i := range out {
		indexByID[out[i].ID] = &out[i]
	}

	aRows, err := r.db.Query(ctx, listAttrsSQL, ids)
	if err != nil {
		return nil, fmt.Errorf("query order_attrs: %w", err)
	}
	for aRows.Next() {
		var orderID string
		var attr order.Attr
		if err := aRows.Scan(&orderID, &attr.Name, &attr.Value, &attr.Price); err != nil {
			aRows.Close()
			return nil, fmt.Errorf("scan order_attr: %w", err)
		}
		if rec, ok := indexByID[orderID]; ok {
			rec.Attrs = append(rec.Attrs, attr)
		}
	}
	aRows.Close()
	if err := aRows.Err(); err != nil {
		return nil, fmt.Errorf("iter order_attrs: %w", err)
	}

	hRows, err := r.db.Query(ctx, listHistorySQL, ids)
	if err != nil {
		return nil, fmt.Errorf("query order_history: %w", err)
	}
	for hRows.Next() {
		var entry order.HistoryEntry
		var orderID string
		var note pgtype.Text
		if err := hRows.Scan(&entry.ID, &orderID, &entry.Status, &note, &entry.CreatedAt); err != nil {
			hRows.Close()
			return nil, fmt.Errorf("scan order_history: %w", err)
		}
		if note.Valid {
			v := note.String
			entry.Note = &v
		}
		if rec, ok := indexByID[orderID]; ok {
			rec.History = append(rec.History, entry)
		}
	}
	hRows.Close()
	if err := hRows.Err(); err != nil {
		return nil, fmt.Errorf("iter order_history: %w", err)
	}

	iRows, err := r.db.Query(ctx, listInvoicesSQL, ids)
	if err != nil {
		return nil, fmt.Errorf("query order_invoices: %w", err)
	}
	for iRows.Next() {
		var orderID string
		var inv order.Invoice
		if err := iRows.Scan(&orderID, &inv.Provider, &inv.ID, &inv.PageURL, &inv.Amount, &inv.Currency); err != nil {
			iRows.Close()
			return nil, fmt.Errorf("scan order_invoice: %w", err)
		}
		if rec, ok := indexByID[orderID]; ok {
			rec.Invoices = append(rec.Invoices, inv)
		}
	}
	iRows.Close()
	if err := iRows.Err(); err != nil {
		return nil, fmt.Errorf("iter order_invoices: %w", err)
	}

	return out, nil
}

// GetStatus returns the current order_status for orderID. Returns
// order.ErrNotFound when no row matches.
func (r *Reader) GetStatus(ctx context.Context, orderID string) (string, error) {
	var status string
	err := r.db.QueryRow(ctx, getOrderStatusSQL, orderID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", order.ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("query order status: %w", err)
	}
	return status, nil
}

const getOrderByIDSQL = `SELECT id::text, product_id, status::text, email, price, currency, lang,
	first_name, middle_name, last_name,
	country, city, phone, address,
	admin_note, customer_note,
	created_at, updated_at
FROM orders
WHERE id = $1::uuid`

// GetByID returns one order populated with its attrs, history, and invoices.
// Returns order.ErrNotFound when no row matches.
func (r *Reader) GetByID(ctx context.Context, id string) (*order.Record, error) {
	var rec order.Record
	var middleName, adminNote, customerNote pgtype.Text
	err := r.db.QueryRow(ctx, getOrderByIDSQL, id).Scan(
		&rec.ID, &rec.ProductID, &rec.Status, &rec.Email, &rec.Price, &rec.Currency, &rec.Lang,
		&rec.FirstName, &middleName, &rec.LastName,
		&rec.Country, &rec.City, &rec.Phone, &rec.Address,
		&adminNote, &customerNote,
		&rec.CreatedAt, &rec.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, order.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query order: %w", err)
	}
	if middleName.Valid {
		v := middleName.String
		rec.MiddleName = &v
	}
	if adminNote.Valid {
		v := adminNote.String
		rec.AdminNote = &v
	}
	if customerNote.Valid {
		v := customerNote.String
		rec.CustomerNote = &v
	}
	rec.Attrs = []order.Attr{}
	rec.History = []order.HistoryEntry{}
	rec.Invoices = []order.Invoice{}

	ids := []string{rec.ID}

	aRows, err := r.db.Query(ctx, listAttrsSQL, ids)
	if err != nil {
		return nil, fmt.Errorf("query order_attrs: %w", err)
	}
	for aRows.Next() {
		var orderID string
		var attr order.Attr
		if err := aRows.Scan(&orderID, &attr.Name, &attr.Value, &attr.Price); err != nil {
			aRows.Close()
			return nil, fmt.Errorf("scan order_attr: %w", err)
		}
		rec.Attrs = append(rec.Attrs, attr)
	}
	aRows.Close()
	if err := aRows.Err(); err != nil {
		return nil, fmt.Errorf("iter order_attrs: %w", err)
	}

	hRows, err := r.db.Query(ctx, listHistorySQL, ids)
	if err != nil {
		return nil, fmt.Errorf("query order_history: %w", err)
	}
	for hRows.Next() {
		var entry order.HistoryEntry
		var orderID string
		var note pgtype.Text
		if err := hRows.Scan(&entry.ID, &orderID, &entry.Status, &note, &entry.CreatedAt); err != nil {
			hRows.Close()
			return nil, fmt.Errorf("scan order_history: %w", err)
		}
		if note.Valid {
			v := note.String
			entry.Note = &v
		}
		rec.History = append(rec.History, entry)
	}
	hRows.Close()
	if err := hRows.Err(); err != nil {
		return nil, fmt.Errorf("iter order_history: %w", err)
	}

	iRows, err := r.db.Query(ctx, listInvoicesSQL, ids)
	if err != nil {
		return nil, fmt.Errorf("query order_invoices: %w", err)
	}
	for iRows.Next() {
		var orderID string
		var inv order.Invoice
		if err := iRows.Scan(&orderID, &inv.Provider, &inv.ID, &inv.PageURL, &inv.Amount, &inv.Currency); err != nil {
			iRows.Close()
			return nil, fmt.Errorf("scan order_invoice: %w", err)
		}
		rec.Invoices = append(rec.Invoices, inv)
	}
	iRows.Close()
	if err := iRows.Err(); err != nil {
		return nil, fmt.Errorf("iter order_invoices: %w", err)
	}

	return &rec, nil
}

// RecordInvoiceEvent persists evt and recomputes the order's payment status
// from the latest invoice event for the order. All writes share a single
// transaction with the orders row locked FOR UPDATE so concurrent webhook
// deliveries for the same order are serialized.
//
// Idempotent on (invoice_id, provider, status, event_at): a duplicate webhook
// re-derives from the unchanged latest event and is a no-op.
//
// Returns order.ErrNotFound when the order does not exist.
func (w *Writer) RecordInvoiceEvent(ctx context.Context, evt order.InvoiceEvent) (string, error) {
	if len(evt.Payload) == 0 {
		return "", fmt.Errorf("invoice event payload is required")
	}

	tx, err := w.db.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		// Rollback after Commit is a no-op (returns ErrTxClosed); ignore it.
		if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
			_ = rbErr
		}
	}()

	var currentStatus string
	if err := tx.QueryRow(ctx, lockOrderStatusSQL, evt.OrderID).Scan(&currentStatus); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", order.ErrNotFound
		}
		return "", fmt.Errorf("lock order: %w", err)
	}

	var noteArg any = evt.Note
	if evt.Note == "" {
		noteArg = nil
	}
	if _, err := tx.Exec(ctx, insertInvoiceHistorySQL,
		evt.OrderID, evt.InvoiceID, evt.Provider, evt.Status, noteArg, []byte(evt.Payload), evt.EventAt,
	); err != nil {
		return "", fmt.Errorf("insert invoice_history: %w", err)
	}

	var latestStatus string
	var latestNote pgtype.Text
	if err := tx.QueryRow(ctx, latestInvoiceEventSQL, evt.OrderID).Scan(&latestStatus, &latestNote); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// No event survived the insert (duplicate filtered AND no prior rows).
			// This shouldn't happen — the unique constraint allows the very first
			// (orderID, invoice, status, event_at) tuple — but bail safely.
			return "", tx.Commit(ctx)
		}
		return "", fmt.Errorf("query latest invoice_history: %w", err)
	}

	candidate, ok := order.InvoiceStatusToOrderStatus(latestStatus)
	if !ok || !order.ShouldApplyInvoiceTransition(currentStatus, candidate) {
		return "", tx.Commit(ctx)
	}

	tag, err := tx.Exec(ctx, updateOrderStatusSQL, evt.OrderID, candidate)
	if err != nil {
		return "", fmt.Errorf("update order status: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return "", fmt.Errorf("update order status: expected 1 row affected, got %d", tag.RowsAffected())
	}

	var orderNoteArg any
	if latestNote.Valid {
		orderNoteArg = latestNote.String
	}
	if _, err := tx.Exec(ctx, insertOrderHistoryNoteSQL, evt.OrderID, candidate, orderNoteArg); err != nil {
		return "", fmt.Errorf("insert order_history: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit tx: %w", err)
	}
	return candidate, nil
}
