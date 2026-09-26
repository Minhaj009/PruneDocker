# Multi-Stage Benchmark Dockerfile
# Stage 1: Heavy Dependency Layer
FROM alpine:3.19 AS deps
WORKDIR /build
# Simulate heavy dependency caching
RUN dd if=/dev/urandom of=deps.bundle bs=1M count=25 2>/dev/null || true
RUN echo "Dependencies compiled into cache" > deps.info

# Stage 2: Intermediate Compilation Stage
FROM alpine:3.19 AS builder
WORKDIR /build
COPY --from=deps /build/deps.bundle ./
COPY --from=deps /build/deps.info ./
RUN echo "Compiling intermediate application binary..." > app.bin

# Stage 3: Production Runner
FROM alpine:3.19 AS runner
WORKDIR /app
COPY --from=builder /build/app.bin ./
CMD ["cat", "app.bin"]
