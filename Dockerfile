# Stage 1: Build
FROM golang:1.24-alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o bookstack-sync ./cmd/bookstack-sync

# Stage 2: Final
FROM alpine:3.21

RUN apk add --no-cache ca-certificates

COPY --from=builder /build/bookstack-sync /usr/local/bin/bookstack-sync

ENTRYPOINT ["/usr/local/bin/bookstack-sync"]
