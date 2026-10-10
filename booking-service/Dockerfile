# ---- Stage 1: build ----
# Keep this version in sync with the "go" line in go.mod.
FROM golang:1.25 AS build
WORKDIR /src

# Download dependencies first so Docker can cache this layer.
COPY go.mod go.sum ./
RUN go mod download

COPY gen ./gen
COPY booking-service ./booking-service
RUN CGO_ENABLED=0 go build -o /out/booking-service ./booking-service

# ---- Stage 2: tiny runtime image ----
FROM gcr.io/distroless/static-debian12
COPY --from=build /out/booking-service /booking-service
USER nonroot
ENTRYPOINT ["/booking-service"]
