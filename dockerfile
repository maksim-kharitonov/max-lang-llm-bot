# Dockerfile
FROM golang:1.25.3-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o max-lang-llm-bot ./cmd/max-lang-llm-bot/main.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/max-lang-llm-bot .
EXPOSE 1984
CMD ["./max-lang-llm-bot"]
