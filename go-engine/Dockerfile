# ---- build stage ----
FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/go-engine ./cmd/server

# ---- runtime stage ----
FROM alpine:3.19

RUN apk add --no-cache ca-certificates

WORKDIR /app
COPY --from=builder /bin/go-engine /app/go-engine
COPY migrations ./migrations

EXPOSE 8080

ENTRYPOINT ["/app/go-engine"]