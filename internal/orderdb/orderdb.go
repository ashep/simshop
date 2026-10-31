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

// Writer inserts an order with its items and their attributes into PostgreSQL
// inside a single transaction so partial failure cannot leave an order without
// its items (or vice versa). Implements order.Writer and order.InvoiceWriter.
type Writer struct {
	db *pgxpool.Pool
}

// New returns a Writer backed by db.
func New(db *pgxpool.Pool) *Writer {
	return &Writer{db: db}
}

const insertOrderSQL = `INSERT INTO orders (
	email, price, currency, lang,
	first_name, middle_name, last_name,
	country, city, phone, address,
	customer_note,
	promo_code, discount_percent, discount,
	ga_client_id, ga_session_id,
	meta_fbp, meta_fbc, client_ip, user_agent
) VALUES (
	$1, $2, $3, $4,
	$5, $6, $7,
	$8, $9, $10, $11,
	$12,
	$13, $14, $15,
	$16, $17,
	$18, $19, $20, $21
) RETURNING id`

// lockPromoPhoneSQL serializes concurrent orders carrying the same promo code
// and phone number, so both cannot pass promoUsedSQL before either commits.
const lockPromoPhoneSQL = `SELECT pg_advisory_xact_lock(
	hashtextextended(lower($1) || ':' || regexp_replace($2, '\D', '', 'g'), 0))`

// promoUsedSQL counts a code as used by any order with the same phone digits,
// except cancelled ones (failed/expired payment, operator abort), so the
// customer can retry with the code after a failed payment. 'new' counts too:
// it is the state of a concurrent order between its tx1 and tx2.
const promoUsedSQL = `SELECT EXISTS (
	SELECT 1 FROM orders
	WHERE promo_code IS NOT NULL
	  AND lower(promo_code) = lower($1)
	  AND regexp_replace(phone, '\D', '', 'g') = regexp_replace($2, '\D', '', 'g')
	  AND status <> 'cancelled')`

const insertItemSQL = `INSERT INTO order_items
	(order_id, position, product_id, qty, price, discount)
	VALUES ($1, $2, $3, $4, $5, $6)
	RETURNING id`

const insertAttrSQL = `INSERT INTO order_attrs
	(order_id, item_id, attr_name, attr_value, attr_price)
	VALUES ($1, $2, $3, $4, $5)`

const insertHistorySQL = `INSERT INTO order_history (order_id, status) VALUES ($1, 'new')`

const insertInvoiceSQL = `INSERT INTO order_invoices
	(order_id, provider, id, page_url, amount, currency)
	VALUES ($1, $2, $3, $4, $5, $6)`

const updateOrderStatusAwaitingPaymentSQL = `UPDATE orders
	SET status = 'awaiting_payment', updated_at = CURRENT_TIMESTAMP
	WHERE id = $1`

const insertHistoryAwaitingPaymentSQL = `INSERT INTO order_history
	(order_id, status) VALUES ($1, 'awaiting_payment')`

// Write inserts a row for o into orders, one row per item into order_items and
// one row per item attr into order_attrs,
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

	var promoCode, discountPercent, discount any
	if o.Promo != nil {
		promoCode, discountPercent, discount = o.Promo.Code, o.Promo.Percent, o.Promo.Discount

		if _, err := tx.Exec(ctx, lockPromoPhoneSQL, o.Promo.Code, o.Phone); err != nil {
			return "", fmt.Errorf("lock promo code: %w", err)
		}
		var used bool
		if err := tx.QueryRow(ctx, promoUsedSQL, o.Promo.Code, o.Phone).Scan(&used); err != nil {
			return "", fmt.Errorf("check promo code usage: %w", err)
		}
		if used {
			return "", order.ErrPromoCodeUsed
		}
	}

	var orderID string
	if err := tx.QueryRow(ctx, insertOrderSQL,
		o.Email, o.Price, o.Currency, o.Lang,
		o.FirstName, nullIfEmpty(o.MiddleName), o.LastName,
		o.Country, o.City, o.Phone, o.Address,
		nullIfEmpty(o.CustomerNote),
		promoCode, discountPercent, discount,
		nullIfEmpty(o.GAClientID), nullIfEmpty(o.GASessionID),
		nullIfEmpty(o.MetaFBP), nullIfEmpty(o.MetaFBC), nullIfEmpty(o.ClientIP), nullIfEmpty(o.UserAgent),
	).Scan(&orderID); err != nil {
		return "", fmt.Errorf("insert order: %w", err)
	}

	for pos, it := range o.Items {
		var itemID string
		if err := tx.QueryRow(ctx, insertItemSQL,
			orderID, pos, it.ProductID, it.Qty, it.Price, it.Discount,
		).Scan(&itemID); err != nil {
			return "", fmt.Errorf("insert order_item %q: %w", it.ProductID, err)
		}
		for _, a := range it.Attrs {
			if _, err := tx.Exec(ctx, insertAttrSQL, orderID, itemID, a.Name, a.Value, a.Price); err != nil {
				return "", fmt.Errorf("insert order_attr %q: %w", a.Name, err)
			}
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
// to status='awaiting_payment' in a single transaction. Order, items, and the
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

const listOrdersSQL = `SELECT id::text, status::text, email, price, currency, lang,
	first_name, middle_name, last_name,
	country, city, phone, address,
	admin_note, customer_note, tracking_number,
	created_at, updated_at,
	promo_code, discount_percent, discount,
	ga_client_id, ga_session_id,
	meta_fbp, meta_fbc, client_ip, user_agent
FROM orders
ORDER BY created_at DESC`

const listOrdersByStatusSQL = `SELECT id::text, status::text, email, price, currency, lang,
	first_name, middle_name, last_name,
	country, city, phone, address,
	admin_note, customer_note, tracking_number,
	created_at, updated_at,
	promo_code, discount_percent, discount,
	ga_client_id, ga_session_id,
	meta_fbp, meta_fbc, client_ip, user_agent
FROM orders
WHERE status = ANY($1::order_status[])
ORDER BY created_at DESC`

const listItemsSQL = `SELECT id::text, order_id::text, product_id, qty, price, discount
FROM order_items
WHERE order_id = ANY($1::uuid[])
ORDER BY order_id, position`

const listAttrsSQL = `SELECT item_id::text, attr_name, attr_value, attr_price
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

const updateOrderStatusByOperatorSQL = `UPDATE orders
    SET status = $2::order_status,
        updated_at = CURRENT_TIMESTAMP,
        tracking_number = CASE WHEN $3 = '' THEN tracking_number ELSE $3 END
    WHERE id = $1::uuid`

const insertInvoiceHistorySQL = `INSERT INTO invoice_history
	(order_id, invoice_id, provider, status, note, payload, event_at)
	VALUES ($1::uuid, $2, $3, $4::invoice_status, $5, $6, $7)
	ON CONFLICT (invoice_id, provider, status, event_at) DO NOTHING`

const latestInvoiceEventSQL = `SELECT status::text, note FROM invoice_history
	WHERE order_id = $1::uuid
	ORDER BY event_at DESC, created_at DESC
	LIMIT 1`

// List returns all orders, newest first, each populated with its items and
// history. When statuses is non-empty, only orders whose order_status matches
// one of the supplied values are returned. Always returns a non-nil slice on
// success (possibly empty).
func (r *Reader) List(ctx context.Context, statuses []string) ([]order.Record, error) {
	var rows pgx.Rows
	var err error
	if len(statuses) == 0 {
		rows, err = r.db.Query(ctx, listOrdersSQL)
	} else {
		rows, err = r.db.Query(ctx, listOrdersByStatusSQL, statuses)
	}
	if err != nil {
		return nil, fmt.Errorf("query orders: %w", err)
	}

	out := []order.Record{}
	var ids []string
	for rows.Next() {
		var rec order.Record
		var middleName, adminNote, customerNote, trackingNumber, promoCode, gaClientID, gaSessionID,
			metaFBP, metaFBC, clientIP, userAgent pgtype.Text
		var discountPercent, discount pgtype.Int4
		if err := rows.Scan(
			&rec.ID, &rec.Status, &rec.Email, &rec.Price, &rec.Currency, &rec.Lang,
			&rec.FirstName, &middleName, &rec.LastName,
			&rec.Country, &rec.City, &rec.Phone, &rec.Address,
			&adminNote, &customerNote, &trackingNumber,
			&rec.CreatedAt, &rec.UpdatedAt,
			&promoCode, &discountPercent, &discount,
			&gaClientID, &gaSessionID,
			&metaFBP, &metaFBC, &clientIP, &userAgent,
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
		if trackingNumber.Valid {
			v := trackingNumber.String
			rec.TrackingNumber = &v
		}
		rec.Promo = scanPromo(promoCode, discountPercent, discount)
		rec.GAClientID, rec.GASessionID = gaClientID.String, gaSessionID.String
		rec.MetaFBP, rec.MetaFBC = metaFBP.String, metaFBC.String
		rec.ClientIP, rec.UserAgent = clientIP.String, userAgent.String
		rec.Items = []order.Item{}
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

	if err := r.loadItems(ctx, ids, indexByID); err != nil {
		return nil, err
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

// loadItems fills Items (with their attrs) of the records in byID, whose keys
// are ids.
func (r *Reader) loadItems(ctx context.Context, ids []string, byID map[string]*order.Record) error {
	rows, err := r.db.Query(ctx, listItemsSQL, ids)
	if err != nil {
		return fmt.Errorf("query order_items: %w", err)
	}
	type itemRef struct {
		rec *order.Record
		idx int
	}
	items := map[string]itemRef{}
	for rows.Next() {
		var itemID, orderID string
		var it order.Item
		if err := rows.Scan(&itemID, &orderID, &it.ProductID, &it.Qty, &it.Price, &it.Discount); err != nil {
			rows.Close()
			return fmt.Errorf("scan order_item: %w", err)
		}
		it.Attrs = []order.Attr{}
		if rec, ok := byID[orderID]; ok {
			rec.Items = append(rec.Items, it)
			items[itemID] = itemRef{rec: rec, idx: len(rec.Items) - 1}
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iter order_items: %w", err)
	}

	aRows, err := r.db.Query(ctx, listAttrsSQL, ids)
	if err != nil {
		return fmt.Errorf("query order_attrs: %w", err)
	}
	for aRows.Next() {
		var itemID string
		var attr order.Attr
		if err := aRows.Scan(&itemID, &attr.Name, &attr.Value, &attr.Price); err != nil {
			aRows.Close()
			return fmt.Errorf("scan order_attr: %w", err)
		}
		if ref, ok := items[itemID]; ok {
			ref.rec.Items[ref.idx].Attrs = append(ref.rec.Items[ref.idx].Attrs, attr)
		}
	}
	aRows.Close()
	if err := aRows.Err(); err != nil {
		return fmt.Errorf("iter order_attrs: %w", err)
	}
	return nil
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

const getOrderByIDSQL = `SELECT id::text, status::text, email, price, currency, lang,
	first_name, middle_name, last_name,
	country, city, phone, address,
	admin_note, customer_note, tracking_number,
	created_at, updated_at,
	promo_code, discount_percent, discount,
	ga_client_id, ga_session_id,
	meta_fbp, meta_fbc, client_ip, user_agent
FROM orders
WHERE id = $1::uuid`

// GetByID returns one order populated with its items, history, and invoices.
// Returns order.ErrNotFound when no row matches.
func (r *Reader) GetByID(ctx context.Context, id string) (*order.Record, error) {
	var rec order.Record
	var middleName, adminNote, customerNote, trackingNumber, promoCode, gaClientID, gaSessionID,
		metaFBP, metaFBC, clientIP, userAgent pgtype.Text
	var discountPercent, discount pgtype.Int4
	err := r.db.QueryRow(ctx, getOrderByIDSQL, id).Scan(
		&rec.ID, &rec.Status, &rec.Email, &rec.Price, &rec.Currency, &rec.Lang,
		&rec.FirstName, &middleName, &rec.LastName,
		&rec.Country, &rec.City, &rec.Phone, &rec.Address,
		&adminNote, &customerNote, &trackingNumber,
		&rec.CreatedAt, &rec.UpdatedAt,
		&promoCode, &discountPercent, &discount,
		&gaClientID, &gaSessionID,
		&metaFBP, &metaFBC, &clientIP, &userAgent,
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
	if trackingNumber.Valid {
		v := trackingNumber.String
		rec.TrackingNumber = &v
	}
	rec.Promo = scanPromo(promoCode, discountPercent, discount)
	rec.GAClientID, rec.GASessionID = gaClientID.String, gaSessionID.String
	rec.MetaFBP, rec.MetaFBC = metaFBP.String, metaFBC.String
	rec.ClientIP, rec.UserAgent = clientIP.String, userAgent.String
	rec.Items = []order.Item{}
	rec.History = []order.HistoryEntry{}
	rec.Invoices = []order.Invoice{}

	ids := []string{rec.ID}

	if err := r.loadItems(ctx, ids, map[string]*order.Record{rec.ID: &rec}); err != nil {
		return nil, err
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

// UpdateStatusByOperator applies an operator-driven status transition to an
// order in a single transaction. Locks the orders row FOR UPDATE so the rule
// re-check is authoritative against concurrent webhook and operator writers.
//
// Returns:
//
//   - (true, nil) — UPDATE + INSERT applied.
//   - (false, nil) — under the lock the order was already at target
//     (concurrent same-target writer beat us). No write performed.
//   - (false, order.ErrNotFound) — no order row.
//   - (false, order.ErrTransitionNotAllowed) — concurrent change put the
//     order in a state that no longer permits target.
//   - (false, error) — DB error.
//
// trackingNumber is written only when non-empty; empty leaves the column
// untouched (CASE in updateOrderStatusByOperatorSQL).
func (w *Writer) UpdateStatusByOperator(
	ctx context.Context,
	orderID, target, note, trackingNumber string,
) (bool, error) {
	tx, err := w.db.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		// Rollback after Commit is a no-op (returns ErrTxClosed); ignore it.
		if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
			_ = rbErr
		}
	}()

	var current string
	if err := tx.QueryRow(ctx, lockOrderStatusSQL, orderID).Scan(&current); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, order.ErrNotFound
		}
		return false, fmt.Errorf("lock order: %w", err)
	}

	if current == target {
		return false, nil
	}
	if !order.ShouldApplyOperatorTransition(current, target) {
		return false, order.ErrTransitionNotAllowed
	}

	tag, err := tx.Exec(ctx, updateOrderStatusByOperatorSQL, orderID, target, trackingNumber)
	if err != nil {
		return false, fmt.Errorf("update order status: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return false, fmt.Errorf("update order status: expected 1 row affected, got %d", tag.RowsAffected())
	}

	var noteArg any
	if note != "" {
		noteArg = note
	}
	if _, err := tx.Exec(ctx, insertOrderHistoryNoteSQL, orderID, target, noteArg); err != nil {
		return false, fmt.Errorf("insert order_history: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit tx: %w", err)
	}
	return true, nil
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

// scanPromo rebuilds an order's promo from its nullable columns; a NULL
// promo_code means no promo was applied.
func scanPromo(code pgtype.Text, percent, discount pgtype.Int4) *order.Promo {
	if !code.Valid {
		return nil
	}
	return &order.Promo{Code: code.String, Percent: int(percent.Int32), Discount: int(discount.Int32)}
}
