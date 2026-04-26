# syntax=docker/dockerfile:1
FROM golang:1.25-alpine AS build

RUN apk update && apk add --no-cache \
    git \
    gcc \
    musl-dev

ARG GITHUB_TOKEN
RUN echo "machine github.com login porebric password ${GITHUB_TOKEN}" > /root/.netrc && chmod 600 /root/.netrc

ENV GOPRIVATE=github.com

WORKDIR /app

COPY . .

RUN go mod tidy
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -o /out/core-users ./cmd/core-users

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=build /out/core-users .
COPY --from=build /app/config/configs_keys.yml ./config/configs_keys.yml
EXPOSE 5001 9001
ENV APP_ENV=production
CMD ["./core-users"]
