package main

import (
	"errors"
	"fmt"

	pb "trailbooking/gen/trailpb"
)

var validDifficulties = map[string]bool{
	"EASY":     true,
	"MODERATE": true,
	"HARD":     true,
}

// validateTrail checks the business rules for a trail.
// Callers must trim the name and upper-case the difficulty first.
func validateTrail(t *pb.Trail) error {
	if t.Name == "" {
		return errors.New("name is required")
	}
	if len(t.Name) > 100 {
		return errors.New("name must be at most 100 characters")
	}
	if !validDifficulties[t.Difficulty] {
		return fmt.Errorf("difficulty must be one of EASY, MODERATE, HARD (got %q)", t.Difficulty)
	}
	if t.DistanceKm <= 0 {
		return errors.New("distance_km must be greater than 0")
	}
	if t.MaxGroupSize <= 0 {
		return errors.New("max_group_size must be greater than 0")
	}
	return nil
}
