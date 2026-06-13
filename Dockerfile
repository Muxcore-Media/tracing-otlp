FROM golang:1.26-alpine AS builder
COPY core/ /build/core/
COPY tracing-otlp/ /build/tracing-otlp/
WORKDIR /build/tracing-otlp
RUN go mod download && CGO_ENABLED=0 go build -o /tracing-otlp ./cmd/module
FROM alpine:3.21
RUN adduser -D -h /data app
USER app
WORKDIR /app
COPY --from=builder /tracing-otlp .
ENTRYPOINT ["./tracing-otlp"]
