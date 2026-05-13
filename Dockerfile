FROM golang:1.25-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/bin/ms2-os-service ./cmd/api

FROM alpine:3.20

RUN addgroup -S app && adduser -S app -G app
WORKDIR /app
COPY --from=builder /app/bin/ms2-os-service /app/ms2-os-service

USER app
EXPOSE 8082
ENTRYPOINT ["/app/ms2-os-service"]
