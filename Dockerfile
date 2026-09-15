FROM golang:1.25-alpine AS builder
ARG VERSION=0.0.1
ARG BUILD_DATE=2026-09-15

WORKDIR /app
COPY comments-service/go.mod ./
RUN if [ -f comments-service/go.sum ]; then cp comments-service/go.sum .; fi
RUN go mod download
COPY comments-service/. .
RUN CGO_ENABLED=0 GOOS=linux go build \
  -ldflags="-w -s -X main.version=$VERSION -X main.buildDate=$BUILD_DATE" \
  -o comments-server ./cmd/server/main.go

FROM alpine:3.18
ARG VERSION=0.0.1
ARG BUILD_DATE=2026-09-15
LABEL version=$VERSION \
  build-date=$BUILD_DATE \
  maintainer="me@xomrkob.ru"
RUN apk add --no-cache ca-certificates
RUN addgroup -g 1000 appgroup && \
  adduser -D -u 1000 -G appgroup appuser
WORKDIR /app
COPY --from=builder --chown=appuser:appgroup /app/comments-server .
USER appuser
ENTRYPOINT ["/app/comments-server"]