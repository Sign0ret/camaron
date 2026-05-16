FROM golang:1.23-alpine AS builder

WORKDIR /build

COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o orchestrator .

FROM alpine:3.20

RUN apk add --no-cache ca-certificates

COPY --from=builder /build/orchestrator /usr/local/bin/orchestrator

EXPOSE 8080
CMD ["orchestrator"]
