FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o pool ./cmd/pool
RUN CGO_ENABLED=0 GOOS=linux go build -o payout ./cmd/payout
RUN CGO_ENABLED=0 GOOS=linux go build -o api ./cmd/api

FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /app

COPY --from=builder /build/pool /app/pool
COPY --from=builder /build/payout /app/payout
COPY --from=builder /build/api /app/api

EXPOSE 3360 3361 3362 8080

CMD ["/app/pool"]
