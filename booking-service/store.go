package main

import (
	"context"
	"database/sql"
	"errors"

	pb "trailbooking/gen/bookingpb"
)

// errNotFound is returned by the store when a booking does not exist.
var errNotFound = errors.New("not found")

// BookingStore is the only code that talks to the bookings database.
// Note: there is deliberately NO trails table here - trail data belongs to
// Service B and is only reachable through its gRPC API.
type BookingStore struct {
	db *sql.DB
}

func NewBookingStore(db *sql.DB) *BookingStore {
	return &BookingStore{db: db}
}

// Init creates the bookings table if it does not exist yet.
func (s *BookingStore) Init(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS bookings (
			id         TEXT PRIMARY KEY,
			trail_id   TEXT NOT NULL,
			hiker_name TEXT NOT NULL,
			hike_date  TEXT NOT NULL,
			group_size INTEGER NOT NULL
		)`)
	return err
}

func (s *BookingStore) Create(ctx context.Context, b *pb.Booking) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO bookings (id, trail_id, hiker_name, hike_date, group_size)
		 VALUES ($1, $2, $3, $4, $5)`,
		b.Id, b.TrailId, b.HikerName, b.HikeDate, b.GroupSize)
	return err
}

func (s *BookingStore) Get(ctx context.Context, id string) (*pb.Booking, error) {
	b := &pb.Booking{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, trail_id, hiker_name, hike_date, group_size
		 FROM bookings WHERE id = $1`, id).
		Scan(&b.Id, &b.TrailId, &b.HikerName, &b.HikeDate, &b.GroupSize)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errNotFound
	}
	if err != nil {
		return nil, err
	}
	return b, nil
}

// Update changes the editable fields. The trail of a booking cannot be changed.
func (s *BookingStore) Update(ctx context.Context, b *pb.Booking) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE bookings
		 SET hiker_name = $2, hike_date = $3, group_size = $4
		 WHERE id = $1`,
		b.Id, b.HikerName, b.HikeDate, b.GroupSize)
	if err != nil {
		return err
	}
	return requireRowAffected(res)
}

func (s *BookingStore) Delete(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM bookings WHERE id = $1`, id)
	if err != nil {
		return err
	}
	return requireRowAffected(res)
}

func (s *BookingStore) List(ctx context.Context) ([]*pb.Booking, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, trail_id, hiker_name, hike_date, group_size
		 FROM bookings ORDER BY hike_date, hiker_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	bookings := []*pb.Booking{}
	for rows.Next() {
		b := &pb.Booking{}
		if err := rows.Scan(&b.Id, &b.TrailId, &b.HikerName, &b.HikeDate, &b.GroupSize); err != nil {
			return nil, err
		}
		bookings = append(bookings, b)
	}
	return bookings, rows.Err()
}

// requireRowAffected turns "0 rows changed" into errNotFound.
func requireRowAffected(res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return errNotFound
	}
	return nil
}
