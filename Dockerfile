FROM golang:1.25-alpine AS builder
WORKDIR /build

RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /app/core-users ./cmd/core-users

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /app/core-users .
COPY --from=builder /build/config/configs_keys.yml ./config/configs_keys.yml
EXPOSE 5001 9001
CMD ["./core-users"]
