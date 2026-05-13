FROM golang:1.25-alpine AS builder
WORKDIR /app
RUN apk add --no-cache git

# Build the mock from its own minimal go.mod (no shared deps).
COPY go.mod ./
COPY main.go ./
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -a -o bin/mockmp main.go

FROM alpine:3.20
WORKDIR /app
COPY --from=builder /app/bin/mockmp /app/mockmp
EXPOSE 9999
ENTRYPOINT ["/app/mockmp"]
