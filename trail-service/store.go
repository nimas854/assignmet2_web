package main

import (
	"context"
	"database/sql"
	"errors"

	pb "trailbooking/gen/trailpb"
)

// errNotFound is returned by the store when a trail does not exist.
var errNotFound = errors.New("not found")

// TrailStore is the only code that talks to the trails database.
type TrailStore struct {
	db *sql.DB
}

func NewTrailStore(db *sql.DB) *TrailStore {
	return &TrailStore{db: db}
}

// Init creates the trails table if it does not exist yet.
func (s *TrailStore) Init(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS trails (
			id             TEXT PRIMARY KEY,
			name           TEXT NOT NULL,
			difficulty     TEXT NOT NULL,
			distance_km    DOUBLE PRECISION NOT NULL,
			max_group_size INTEGER NOT NULL
		)`)
	return err
}

func (s *TrailStore) Create(ctx context.Context, t *pb.Trail) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO trails (id, name, difficulty, distance_km, max_group_size)
		 VALUES ($1, $2, $3, $4, $5)`,
		t.Id, t.Name, t.Difficulty, t.DistanceKm, t.MaxGroupSize)
	return err
}

func (s *TrailStore) Get(ctx context.Context, id string) (*pb.Trail, error) {
	t := &pb.Trail{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, difficulty, distance_km, max_group_size
		 FROM trails WHERE id = $1`, id).
		Scan(&t.Id, &t.Name, &t.Difficulty, &t.DistanceKm, &t.MaxGroupSize)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errNotFound
	}
	if err != nil {
		return nil, err
	}
	return t, nil
}

func (s *TrailStore) Update(ctx context.Context, t *pb.Trail) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE trails
		 SET name = $2, difficulty = $3, distance_km = $4, max_group_size = $5
		 WHERE id = $1`,
		t.Id, t.Name, t.Difficulty, t.DistanceKm, t.MaxGroupSize)
	if err != nil {
		return err
	}
	return requireRowAffected(res)
}

func (s *TrailStore) Delete(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM trails WHERE id = $1`, id)
	if err != nil {
		return err
	}
	return requireRowAffected(res)
}

func (s *TrailStore) List(ctx context.Context) ([]*pb.Trail, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, difficulty, distance_km, max_group_size
		 FROM trails ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	trails := []*pb.Trail{}
	for rows.Next() {
		t := &pb.Trail{}
		if err := rows.Scan(&t.Id, &t.Name, &t.Difficulty, &t.DistanceKm, &t.MaxGroupSize); err != nil {
			return nil, err
		}
		trails = append(trails, t)
	}
	return trails, rows.Err()
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
