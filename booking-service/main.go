// Booking Service (Service A): owns the Booking resource and its own database.
// It calls the Trail Service (Service B) over gRPC, protected by a timeout,
// retries with backoff and a circuit breaker (see trailclient.go).
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"

	pb "trailbooking/gen/bookingpb"
)

// mustEnv reads a required environment variable or stops the program.
func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("missing required environment variable %s", key)
	}
	return v
}

// envInt reads an optional integer environment variable with a default.
func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		log.Fatalf("environment variable %s must be an integer (got %q)", key, v)
	}
	return n
}

// connectDB opens the database and waits until it accepts connections.
func connectDB() *sql.DB {
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		mustEnv("BOOKING_DB_HOST"),
		mustEnv("BOOKING_DB_PORT"),
		mustEnv("BOOKING_DB_USER"),
		mustEnv("BOOKING_DB_PASSWORD"),
		mustEnv("BOOKING_DB_NAME"),
	)
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatalf("cannot open database: %v", err)
	}

	for attempt := 1; attempt <= 30; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		err = db.PingContext(ctx)
		cancel()
		if err == nil {
			return db
		}
		log.Printf("database not ready (attempt %d/30): %v", attempt, err)
		time.Sleep(2 * time.Second)
	}
	log.Fatalf("could not connect to database: %v", err)
	return nil
}

func main() {
	port := mustEnv("BOOKING_SERVICE_PORT")

	db := connectDB()
	store := NewBookingStore(db)
	if err := store.Init(context.Background()); err != nil {
		log.Fatalf("cannot create schema: %v", err)
	}

	// gRPC connection to the Trail Service (Service B).
	// grpc.NewClient connects lazily, so this works even if Service B is down.
	trailConn, err := grpc.NewClient(mustEnv("TRAIL_SERVICE_ADDR"),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("cannot create trail service client: %v", err)
	}
	defer trailConn.Close()

	// Resilience settings (all optional, with sensible defaults).
	cfg := ResilienceConfig{
		Timeout:         time.Duration(envInt("TRAIL_TIMEOUT_MS", 2000)) * time.Millisecond,
		MaxAttempts:     envInt("TRAIL_MAX_ATTEMPTS", 3),
		BaseBackoff:     time.Duration(envInt("TRAIL_BACKOFF_MS", 200)) * time.Millisecond,
		BreakerFailures: uint32(envInt("BREAKER_FAILURES", 3)),
		BreakerOpenFor:  time.Duration(envInt("BREAKER_OPEN_SECONDS", 10)) * time.Second,
	}
	trails := NewTrailClient(trailConn, cfg)

	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("cannot listen on port %s: %v", port, err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterBookingServiceServer(grpcServer, &bookingServer{store: store, trails: trails})
	reflection.Register(grpcServer) // lets Postman / grpcurl discover the API

	go func() {
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
		<-stop
		log.Println("shutting down booking service")
		grpcServer.GracefulStop()
	}()

	log.Printf("booking service listening on :%s (trail service at %s)", port, os.Getenv("TRAIL_SERVICE_ADDR"))
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("gRPC server stopped: %v", err)
	}
}
