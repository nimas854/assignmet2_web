# ---- Stage 1: build ----
# Keep this version in sync with the "go" line in go.mod.
FROM golang:1.25 AS build
WORKDIR /src

# Download dependencies first so Docker can cache this layer.
COPY go.mod go.sum ./
RUN go mod download

COPY gen ./gen
COPY trail-service ./trail-service
RUN CGO_ENABLED=0 go build -o /out/trail-service ./trail-service

# ---- Stage 2: tiny runtime image ----
FROM gcr.io/distroless/static-debian12
COPY --from=build /out/trail-service /trail-service
USER nonroot
ENTRYPOINT ["/trail-service"]
