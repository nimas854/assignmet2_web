package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

var (
	// UseProtoNames keeps field names like distance_km (as in the .proto files).
	// EmitUnpopulated makes empty lists appear as [] instead of vanishing.
	jsonOut = protojson.MarshalOptions{UseProtoNames: true, EmitUnpopulated: true}
	jsonIn  = protojson.UnmarshalOptions{} // unknown fields are rejected -> 400
)

// httpErr is the HTTP status and name used for one gRPC status code.
type httpErr struct {
	status int
	name   string
}

// grpcToHTTP maps gRPC status codes to HTTP status codes.
var grpcToHTTP = map[codes.Code]httpErr{
	codes.InvalidArgument:    {http.StatusBadRequest, "INVALID_ARGUMENT"},
	codes.OutOfRange:         {http.StatusBadRequest, "OUT_OF_RANGE"},
	codes.FailedPrecondition: {http.StatusBadRequest, "FAILED_PRECONDITION"},
	codes.Unauthenticated:    {http.StatusUnauthorized, "UNAUTHENTICATED"},
	codes.PermissionDenied:   {http.StatusForbidden, "PERMISSION_DENIED"},
	codes.NotFound:           {http.StatusNotFound, "NOT_FOUND"},
	codes.AlreadyExists:      {http.StatusConflict, "ALREADY_EXISTS"},
	codes.Aborted:            {http.StatusConflict, "ABORTED"},
	codes.ResourceExhausted:  {http.StatusTooManyRequests, "RESOURCE_EXHAUSTED"},
	codes.Canceled:           {499, "CANCELLED"},
	codes.Unimplemented:      {http.StatusNotImplemented, "UNIMPLEMENTED"},
	codes.Unavailable:        {http.StatusServiceUnavailable, "UNAVAILABLE"},
	codes.DeadlineExceeded:   {http.StatusGatewayTimeout, "DEADLINE_EXCEEDED"},
	codes.Internal:           {http.StatusInternalServerError, "INTERNAL"},
}

// writeProto sends a protobuf message as JSON.
func writeProto(w http.ResponseWriter, code int, m proto.Message) {
	b, err := jsonOut.Marshal(m)
	if err != nil {
		log.Printf("cannot encode response: %v", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL", "internal server error")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = w.Write(b)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError sends a consistent JSON error body.
func writeError(w http.ResponseWriter, httpCode int, name, message string) {
	writeJSON(w, httpCode, map[string]any{
		"error":  message,
		"code":   name,
		"status": httpCode,
	})
}

// writeGRPCError converts a gRPC error from a downstream service into an
// HTTP response with the matching status code.
func writeGRPCError(w http.ResponseWriter, err error) {
	st, _ := status.FromError(err) // non-gRPC errors become code Unknown
	m, ok := grpcToHTTP[st.Code()]
	if !ok {
		m = httpErr{http.StatusInternalServerError, "INTERNAL"}
	}

	msg := st.Message()
	switch {
	case m.status == http.StatusServiceUnavailable && isTransportError(msg):
		// A service the gateway talks to is unreachable: don't leak connection details.
		log.Printf("upstream unreachable: %v", err)
		msg = "a backend service is currently unreachable, please try again later"
	case m.status >= 500 && m.status != http.StatusServiceUnavailable && m.status != http.StatusGatewayTimeout:
		log.Printf("upstream error: %v", err)
		msg = "internal server error"
	}
	writeError(w, m.status, m.name, msg)
}

func isTransportError(msg string) bool {
	return strings.Contains(msg, "connection error") ||
		strings.Contains(msg, "name resolver") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "dial tcp")
}

// decodeProto reads the JSON request body into a protobuf request message.
// On failure it writes a 400 response itself and returns false.
func decodeProto(w http.ResponseWriter, r *http.Request, m proto.Message) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB limit
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "could not read request body")
		return false
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "request body is required")
		return false
	}
	if err := jsonIn.Unmarshal(body, m); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid JSON body: "+err.Error())
		return false
	}
	return true
}

// statusRecorder remembers the status code so the logger can print it.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// logRequests logs one line per request: method, path, status and duration.
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.Printf("%s %s -> %d (%s)", r.Method, r.URL.Path, rec.status, time.Since(start).Round(time.Millisecond))
	})
}
