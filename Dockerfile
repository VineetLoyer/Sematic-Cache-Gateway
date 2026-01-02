# Build stage
FROM golang:1.21-alpine AS builder

# Install git and ca-certificates (needed for fetching dependencies)
RUN apk add --no-cache git ca-certificates

# Set working directory
WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary with optimizations
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s" \
    -o /gateway \
    ./cmd/gateway

# Final stage - minimal image
FROM alpine:3.19

# Install ca-certificates for HTTPS calls to embedding API
RUN apk add --no-cache ca-certificates

# Create non-root user for security
RUN adduser -D -g '' appuser

# Copy binary from builder
COPY --from=builder /gateway /gateway

# Use non-root user
USER appuser

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Run the gateway
ENTRYPOINT ["/gateway"]
