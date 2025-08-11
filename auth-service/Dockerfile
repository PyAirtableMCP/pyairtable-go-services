FROM golang:1.23-alpine AS builder

RUN apk add --no-cache git

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main cmd/auth-service/main.go

FROM alpine:3.18
RUN apk --no-cache add ca-certificates tzdata curl
WORKDIR /root/

COPY --from=builder /app/main .

EXPOSE 8001
CMD ["./main"]
