package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/google/uuid"
	"github.com/sony/gobreaker/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "trailbooking/gen/bookingpb"
	trailpb "trailbooking/gen/trailpb"
)

// Shown instead of a trail name when Service B cannot answer (fallback response).
const (
	trailNameUnavailable = "unavailable (trail service is down)"
	trailNameDeleted     = "unknown (trail no longer exists)"
)

// bookingServer implements the BookingService gRPC API.
type bookingServer struct {
	pb.UnimplementedBookingServiceServer
	store  *BookingStore
	trails *TrailClient
}

// toStatus converts store errors into gRPC status errors.
func toStatus(err error) error {
	if errors.Is(err, errNotFound) {
		return status.Error(codes.NotFound, "booking not found")
	}
	log.Printf("internal error: %v", err)
	return status.Error(codes.Internal, "internal server error")
}

// lookupTrail asks Service B for a trail and translates failures into
// meaningful gRPC errors (used when we MUST have the trail, e.g. on create).
func (s *bookingServer) lookupTrail(ctx context.Context, trailID string) (*trailpb.Trail, error) {
	trail, err := s.trails.GetTrail(ctx, trailID)
	if err == nil {
		return trail, nil
	}

	// Circuit breaker is open: fail fast.
	if errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests) {
		return nil, status.Error(codes.Unavailable,
			"trail service is temporarily unavailable (circuit breaker open), please try again shortly")
	}

	switch status.Code(err) {
	case codes.NotFound:
		return nil, status.Errorf(codes.NotFound, "trail %q does not exist", trailID)
	case codes.InvalidArgument:
		return nil, status.Error(codes.InvalidArgument, "invalid trail_id")
	case codes.Unavailable, codes.DeadlineExceeded:
		return nil, status.Error(codes.Unavailable,
			"trail service is unavailable, please try again later")
	default:
		log.Printf("unexpected trail service error: %v", err)
		return nil, status.Error(codes.Internal, "internal server error")
	}
}

// trailName returns the trail's name for display. Unlike lookupTrail it never
// fails: if Service B is down we return a fallback text so that reading
// bookings keeps working (graceful degradation). cache avoids asking Service B
// about the same trail twice in one request.
func (s *bookingServer) trailName(ctx context.Context, trailID string, cache map[string]string) string {
	if name, ok := cache[trailID]; ok {
		return name
	}
	name := trailNameUnavailable
	trail, err := s.trails.GetTrail(ctx, trailID)
	switch {
	case err == nil:
		name = trail.Name
	case status.Code(err) == codes.NotFound:
		name = trailNameDeleted
	}
	cache[trailID] = name
	return name
}

func checkGroupSize(groupSize int32, trail *trailpb.Trail) error {
	if groupSize > trail.MaxGroupSize {
		return status.Error(codes.InvalidArgument,
			fmt.Sprintf("group_size %d exceeds the maximum of %d for this trail", groupSize, trail.MaxGroupSize))
	}
	return nil
}

func (s *bookingServer) CreateBooking(ctx context.Context, req *pb.CreateBookingRequest) (*pb.Booking, error) {
	b := &pb.Booking{
		Id:        uuid.NewString(),
		TrailId:   strings.TrimSpace(req.GetTrailId()),
		HikerName: strings.TrimSpace(req.GetHikerName()),
		HikeDate:  strings.TrimSpace(req.GetHikeDate()),
		GroupSize: req.GetGroupSize(),
	}
	if b.TrailId == "" {
		return nil, status.Error(codes.InvalidArgument, "trail_id is required")
	}
	if err := validateBooking(b); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	// Cross-service check: the trail must exist (gRPC call to Service B).
	trail, err := s.lookupTrail(ctx, b.TrailId)
	if err != nil {
		return nil, err
	}
	if err := checkGroupSize(b.GroupSize, trail); err != nil {
		return nil, err
	}

	if err := s.store.Create(ctx, b); err != nil {
		return nil, toStatus(err)
	}
	b.TrailName = trail.Name
	return b, nil
}

func (s *bookingServer) GetBooking(ctx context.Context, req *pb.GetBookingRequest) (*pb.Booking, error) {
	id := strings.TrimSpace(req.GetId())
	if id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	b, err := s.store.Get(ctx, id)
	if err != nil {
		return nil, toStatus(err)
	}
	b.TrailName = s.trailName(ctx, b.TrailId, map[string]string{})
	return b, nil
}

func (s *bookingServer) UpdateBooking(ctx context.Context, req *pb.UpdateBookingRequest) (*pb.Booking, error) {
	updated := &pb.Booking{
		Id:        strings.TrimSpace(req.GetId()),
		HikerName: strings.TrimSpace(req.GetHikerName()),
		HikeDate:  strings.TrimSpace(req.GetHikeDate()),
		GroupSize: req.GetGroupSize(),
	}
	if updated.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	if err := validateBooking(updated); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	existing, err := s.store.Get(ctx, updated.Id)
	if err != nil {
		return nil, toStatus(err)
	}

	// The new group size must still fit the trail, so ask Service B again.
	trail, err := s.lookupTrail(ctx, existing.TrailId)
	if err != nil {
		return nil, err
	}
	if err := checkGroupSize(updated.GroupSize, trail); err != nil {
		return nil, err
	}

	if err := s.store.Update(ctx, updated); err != nil {
		return nil, toStatus(err)
	}
	updated.TrailId = existing.TrailId
	updated.TrailName = trail.Name
	return updated, nil
}

func (s *bookingServer) DeleteBooking(ctx context.Context, req *pb.DeleteBookingRequest) (*pb.DeleteBookingResponse, error) {
	id := strings.TrimSpace(req.GetId())
	if id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	if err := s.store.Delete(ctx, id); err != nil {
		return nil, toStatus(err)
	}
	return &pb.DeleteBookingResponse{}, nil
}

func (s *bookingServer) ListBookings(ctx context.Context, _ *pb.ListBookingsRequest) (*pb.ListBookingsResponse, error) {
	bookings, err := s.store.List(ctx)
	if err != nil {
		return nil, toStatus(err)
	}
	cache := map[string]string{}
	for _, b := range bookings {
		b.TrailName = s.trailName(ctx, b.TrailId, cache)
	}
	return &pb.ListBookingsResponse{Bookings: bookings}, nil
}
