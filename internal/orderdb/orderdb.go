// Package orderdb persists orders to PostgreSQL.
package orderdb

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ashep/simshop/internal/order"
)

// Writer inserts an order and its attributes into PostgreSQL inside a single
// transaction so partial failure cannot leave an order without its attrs (or
// vice versa). Implements order.Writer.
type Writer struct {
	db *pgxpool.Pool
}

// New returns a Writer backed by db.
func New(db *pgxpool.Pool) *Writer {
	return &Writer{db: db}
}

const insertOrderSQL = `INSERT INTO orders (
	product_id, email, price, currency,
	first_name, middle_name, last_name,
	country, city, phone, address,
	customer_note
) VALUES (
	$1, $2, $3, $4,
	$5, $6, $7,
	$8, $9, $10, $11,
	$12
) RETURNING id`

const insertAttrSQL = `INSERT INTO order_attrs
	(order_id, attr_name, attr_value, attr_price)
	VALUES ($1, $2, $3, $4)`

const insertHistorySQL = `INSERT INTO order_history (order_id, status) VALUES ($1, 'new')`

// Write inserts a row for o into orders and one row per attr into order_attrs.
// id, status, and timestamps are populated by database defaults.
func (w *Writer) Write(ctx context.Context, o order.Order) error {
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

	var orderID string
	if err := tx.QueryRow(ctx, insertOrderSQL,
		o.ProductID, o.Email, o.Price, o.Currency,
		o.FirstName, nullIfEmpty(o.MiddleName), o.LastName,
		o.Country, o.City, o.Phone, o.Address,
		nullIfEmpty(o.CustomerNote),
	).Scan(&orderID); err != nil {
		return fmt.Errorf("insert order: %w", err)
	}

	for _, a := range o.Attrs {
		if _, err := tx.Exec(ctx, insertAttrSQL, orderID, a.Name, a.Value, a.Price); err != nil {
			return fmt.Errorf("insert order_attr %q: %w", a.Name, err)
		}
	}

	if _, err := tx.Exec(ctx, insertHistorySQL, orderID); err != nil {
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
