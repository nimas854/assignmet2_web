// Trail Service (Service B): owns the Trail resource and its own database.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	pb "trailbooking/gen/trailpb"
)

// mustEnv reads a required environment variable or stops the program.
func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("missing required environment variable %s", key)
	}
	return v
}

// connectDB opens the database and waits until it accepts connections
// (Postgres can take a few seconds to start inside Docker Compose).
func connectDB() *sql.DB {
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		mustEnv("TRAIL_DB_HOST"),
		mustEnv("TRAIL_DB_PORT"),
		mustEnv("TRAIL_DB_USER"),
		mustEnv("TRAIL_DB_PASSWORD"),
		mustEnv("TRAIL_DB_NAME"),
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
	port := mustEnv("TRAIL_SERVICE_PORT")

	db := connectDB()
	store := NewTrailStore(db)
	if err := store.Init(context.Background()); err != nil {
		log.Fatalf("cannot create schema: %v", err)
	}

	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("cannot listen on port %s: %v", port, err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterTrailServiceServer(grpcServer, &trailServer{store: store})
	reflection.Register(grpcServer) // lets Postman / grpcurl discover the API

	// Graceful shutdown on Ctrl+C or `docker stop`.
	go func() {
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
		<-stop
		log.Println("shutting down trail service")
		grpcServer.GracefulStop()
	}()

	log.Printf("trail service listening on :%s", port)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("gRPC server stopped: %v", err)
	}
}
