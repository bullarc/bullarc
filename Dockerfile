# Stage 1: Build the binary
FROM golang:alpine AS builder

WORKDIR /build

# Download dependencies first (layer caching)
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build a static binary
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w" \
    -trimpath \
    -o bullarc \
    ./cmd/bullarc

# Stage 2: Minimal runtime image
FROM alpine:3.21

# ca-certificates is required for HTTPS calls to Alpaca and Anthropic APIs.
# tzdata provides timezone data used when parsing timestamps.
RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /build/bullarc /usr/local/bin/bullarc

ENTRYPOINT ["bullarc"]
