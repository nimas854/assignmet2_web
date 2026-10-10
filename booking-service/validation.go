package main

import (
	"errors"
	"fmt"
	"time"

	pb "trailbooking/gen/bookingpb"
)

// validateBooking checks the business rules for the editable booking fields.
// Callers must trim the text fields first.
func validateBooking(b *pb.Booking) error {
	if b.HikerName == "" {
		return errors.New("hiker_name is required")
	}
	if len(b.HikerName) > 100 {
		return errors.New("hiker_name must be at most 100 characters")
	}
	if b.HikeDate == "" {
		return errors.New("hike_date is required (format YYYY-MM-DD)")
	}
	date, err := time.Parse("2006-01-02", b.HikeDate)
	if err != nil {
		return fmt.Errorf("hike_date must be a valid date in YYYY-MM-DD format (got %q)", b.HikeDate)
	}
	today := time.Now().UTC().Truncate(24 * time.Hour)
	if date.Before(today) {
		return errors.New("hike_date must not be in the past")
	}
	if b.GroupSize <= 0 {
		return errors.New("group_size must be greater than 0")
	}
	return nil
}
