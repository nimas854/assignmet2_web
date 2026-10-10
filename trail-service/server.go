package main

import (
	"context"
	"errors"
	"log"
	"strings"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "trailbooking/gen/trailpb"
)

// trailServer implements the TrailService gRPC API.
type trailServer struct {
	pb.UnimplementedTrailServiceServer
	store *TrailStore
}

// toStatus converts store errors into gRPC status errors.
// Internal details are logged but never sent to the client.
func toStatus(err error) error {
	if errors.Is(err, errNotFound) {
		return status.Error(codes.NotFound, "trail not found")
	}
	log.Printf("internal error: %v", err)
	return status.Error(codes.Internal, "internal server error")
}

func (s *trailServer) CreateTrail(ctx context.Context, req *pb.CreateTrailRequest) (*pb.Trail, error) {
	trail := &pb.Trail{
		Id:           uuid.NewString(),
		Name:         strings.TrimSpace(req.GetName()),
		Difficulty:   strings.ToUpper(strings.TrimSpace(req.GetDifficulty())),
		DistanceKm:   req.GetDistanceKm(),
		MaxGroupSize: req.GetMaxGroupSize(),
	}
	if err := validateTrail(trail); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if err := s.store.Create(ctx, trail); err != nil {
		return nil, toStatus(err)
	}
	return trail, nil
}

func (s *trailServer) GetTrail(ctx context.Context, req *pb.GetTrailRequest) (*pb.Trail, error) {
	id := strings.TrimSpace(req.GetId())
	if id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	trail, err := s.store.Get(ctx, id)
	if err != nil {
		return nil, toStatus(err)
	}
	return trail, nil
}

func (s *trailServer) UpdateTrail(ctx context.Context, req *pb.UpdateTrailRequest) (*pb.Trail, error) {
	trail := &pb.Trail{
		Id:           strings.TrimSpace(req.GetId()),
		Name:         strings.TrimSpace(req.GetName()),
		Difficulty:   strings.ToUpper(strings.TrimSpace(req.GetDifficulty())),
		DistanceKm:   req.GetDistanceKm(),
		MaxGroupSize: req.GetMaxGroupSize(),
	}
	if trail.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	if err := validateTrail(trail); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if err := s.store.Update(ctx, trail); err != nil {
		return nil, toStatus(err)
	}
	return trail, nil
}

func (s *trailServer) DeleteTrail(ctx context.Context, req *pb.DeleteTrailRequest) (*pb.DeleteTrailResponse, error) {
	id := strings.TrimSpace(req.GetId())
	if id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	if err := s.store.Delete(ctx, id); err != nil {
		return nil, toStatus(err)
	}
	return &pb.DeleteTrailResponse{}, nil
}

func (s *trailServer) ListTrails(ctx context.Context, _ *pb.ListTrailsRequest) (*pb.ListTrailsResponse, error) {
	trails, err := s.store.List(ctx)
	if err != nil {
		return nil, toStatus(err)
	}
	return &pb.ListTrailsResponse{Trails: trails}, nil
}
