# ===========================================
# Stage 1: 构建阶段
# ===========================================
FROM golang:1.26.10-alpine AS builder

ENV GOTOOLCHAIN=local

ARG VERSION=0.0.0-dev

WORKDIR /app

COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-w -s -X github.com/starcat-app/starcat-recommend-api/internal/version.Version=${VERSION}" \
    -o /app/bin/server \
    ./cmd/server/

# ===========================================
# Stage 2: 运行阶段
# ===========================================
FROM alpine:3.20

RUN apk --no-cache add ca-certificates tzdata wget

ENV TZ=UTC

RUN addgroup -S app && adduser -S app -G app

WORKDIR /app
COPY --from=builder /app/bin/server /app/server

USER app

EXPOSE 5005

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget --quiet --tries=1 --spider http://localhost:5005/healthz || exit 1

ENTRYPOINT ["/app/server"]
