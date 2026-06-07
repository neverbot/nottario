# syntax=docker/dockerfile:1.7

FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown
RUN CGO_ENABLED=0 GOOS=linux \
    go build -trimpath \
      -ldflags "-s -w \
        -X github.com/neverbot/nottario/internal/version.Version=${VERSION} \
        -X github.com/neverbot/nottario/internal/version.Commit=${COMMIT} \
        -X github.com/neverbot/nottario/internal/version.Date=${DATE}" \
      -o /out/nottario ./cmd/nottario

FROM alpine:3.21
# postgresql17-client matches the version used by self-hosters on
# Postgres 17 servers (pg_dump refuses to dump a server newer than
# itself). The pg17 client is backward-compatible with older servers,
# so it also works against the Postgres 16 dev container.
RUN apk add --no-cache postgresql17-client ca-certificates \
    && addgroup -S nonroot \
    && adduser -S -G nonroot -u 65532 -h /home/nonroot nonroot
COPY --from=build /out/nottario /nottario
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/nottario"]
