# ---- Build Stage ----
FROM golang:1.22-alpine AS builder

WORKDIR /build

COPY app/ .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o server main.go

# ---- Runtime Stage ----
FROM alpine:3.19

RUN apk add --no-cache ca-certificates && addgroup -S appuser && adduser -S appuser -G appuser

COPY --from=builder /build/server /usr/local/bin/server

USER appuser

ENV PORT=8080
EXPOSE ${PORT}

ENTRYPOINT ["/usr/local/bin/server"]
