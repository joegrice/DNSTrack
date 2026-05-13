FROM golang:1.26-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o dnstrack ./cmd/dnstrack/

FROM alpine:latest

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /build/dnstrack .
COPY --from=builder /build/web/dist ./web/dist
COPY --from=builder /build/config.yaml .

RUN mkdir -p /app/data

EXPOSE 8080

ENTRYPOINT ["./dnstrack"]
CMD ["-config", "/app/config.yaml", "-db", "/app/data/dnstrack.db", "-frontend", "/app/web/dist"]
