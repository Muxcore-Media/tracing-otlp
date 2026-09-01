# Build from the MuxCore workspace root:
#   docker build -f _mvp/dockerfiles/module.Dockerfile --build-arg MODULE=tracing-otlp -t muxcore/tracing-otlp .
ARG MODULE=tracing-otlp
ARG GO_VERSION=1.26

FROM golang:${GO_VERSION}-alpine AS builder
ARG MODULE
RUN apk add --no-cache git ca-certificates
WORKDIR /src
COPY . .
WORKDIR /src/${MODULE}
RUN test -n "$MODULE" && test -d "/src/${MODULE}"
RUN go mod download
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /tracing-otlp ./cmd/module

FROM alpine:3.21
RUN apk add --no-cache ca-certificates && adduser -D -h /data app
USER app
WORKDIR /app
COPY --from=builder /tracing-otlp .
ENTRYPOINT ["./tracing-otlp"]
