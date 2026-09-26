# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /app
RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/prunedocker ./cmd/prunedocker

# Runtime stage: ultra-lightweight secure alpine container
FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata
WORKDIR /

COPY --from=builder /app/prunedocker /usr/local/bin/prunedocker

# Default Prometheus metrics port
EXPOSE 9199

ENTRYPOINT ["prunedocker"]
CMD ["daemon", "--max-disk-usage", "80", "--check-interval", "30m", "--metrics-addr", ":9199"]
