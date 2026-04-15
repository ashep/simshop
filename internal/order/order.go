package order

import (
	"context"
	"time"
)

type Status string

const (
	StatusNew Status = "New"
)

// Order holds all data for a single customer order.
type Order struct {
	ID          string
	Status      Status
	DateTime    time.Time
	ProductName string
	Attributes  string // formatted: "AttrTitle: ValueTitle, ..." sorted by attribute key
	Price       float64
	Currency    string
	FirstName   string
	MiddleName  string
	LastName    string
	Phone       string
	Email       string
	City        string
	Address     string
	Notes       string
}

// Writer persists an order to an external store.
type Writer interface {
	Write(ctx context.Context, o Order) error
}

// Service submits orders via a Writer.
type Service struct {
	w Writer
}

// NewService returns a Service backed by w.
func NewService(w Writer) *Service {
	return &Service{w: w}
}

// Submit writes the order to the backing store.
func (s *Service) Submit(ctx context.Context, o Order) error {
	return s.w.Write(ctx, o)
}
