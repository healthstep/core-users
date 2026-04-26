FROM golang:1.25-alpine AS build
WORKDIR /app

RUN apk update && apk add --no-cache \
    git \
    gcc \
    musl-dev

ARG GITHUB_TOKEN
RUN echo "machine github.com login porebric password ${GITHUB_TOKEN}" > /root/.netrc && chmod 600 /root/.netrc

ENV GOPRIVATE=github.com

COPY . .

RUN go mod tidy
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -o /app/core-users ./cmd/core-users

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=build /app/core-users .
COPY --from=build /app/config/configs_keys.yml ./config/configs_keys.yml
EXPOSE 5001 9001
ENV APP_ENV=production
CMD ["./core-users"]
