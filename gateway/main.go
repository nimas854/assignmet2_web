// API Gateway: the single REST entry point. It translates REST requests into
// gRPC calls to the Trail Service and Booking Service and translates gRPC
// errors back into HTTP status codes.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	bookingpb "trailbooking/gen/bookingpb"
	trailpb "trailbooking/gen/trailpb"
)

// Gateway holds the gRPC clients used by the HTTP handlers.
type Gateway struct {
	trails   trailpb.TrailServiceClient
	bookings bookingpb.BookingServiceClient
	timeout  time.Duration // max time for one gRPC call made on behalf of a request
}

// ctx creates the context for one downstream gRPC call.
func (g *Gateway) ctx(r *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), g.timeout)
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("missing required environment variable %s", key)
	}
	return v
}

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

func dial(addr string) *grpc.ClientConn {
	// grpc.NewClient connects lazily, so the gateway starts even if a service is down.
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("cannot create gRPC client for %s: %v", addr, err)
	}
	return conn
}

func main() {
	port := mustEnv("GATEWAY_PORT")

	trailConn := dial(mustEnv("TRAIL_SERVICE_ADDR"))
	defer trailConn.Close()
	bookingConn := dial(mustEnv("BOOKING_SERVICE_ADDR"))
	defer bookingConn.Close()

	g := &Gateway{
		trails:   trailpb.NewTrailServiceClient(trailConn),
		bookings: bookingpb.NewBookingServiceClient(bookingConn),
		// Must be longer than the worst-case retry time inside the Booking Service.
		timeout: time.Duration(envInt("GATEWAY_TIMEOUT_SECONDS", 10)) * time.Second,
	}

	mux := http.NewServeMux()

	// Resource-based URLs: nouns in the path, the HTTP method is the action.
	mux.HandleFunc("POST /trails", g.createTrail)
	mux.HandleFunc("GET /trails", g.listTrails)
	mux.HandleFunc("GET /trails/{id}", g.getTrail)
	mux.HandleFunc("PUT /trails/{id}", g.updateTrail)
	mux.HandleFunc("DELETE /trails/{id}", g.deleteTrail)

	mux.HandleFunc("POST /bookings", g.createBooking)
	mux.HandleFunc("GET /bookings", g.listBookings)
	mux.HandleFunc("GET /bookings/{id}", g.getBooking)
	mux.HandleFunc("PUT /bookings/{id}", g.updateBooking)
	mux.HandleFunc("DELETE /bookings/{id}", g.deleteBooking)

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "no such endpoint: "+r.Method+" "+r.URL.Path)
	})

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           logRequests(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Graceful shutdown on Ctrl+C or `docker stop`.
	stop, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	go func() {
		<-stop.Done()
		log.Println("shutting down gateway")
		shutdownCtx, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		_ = srv.Shutdown(shutdownCtx)
	}()

	log.Printf("API gateway listening on :%s", port)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("HTTP server stopped: %v", err)
	}
}
