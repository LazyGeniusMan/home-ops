# syntax=docker/dockerfile:1
# apprise-go-api: stateless-only Go port of Python apprise-api.

FROM golang:1.26.7@sha256:e30143be198ab04cf7ba25fba83ab3a692ca584c994aad0bf131fa0eb32dd8c1 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# VERSION surfaces as apprise_go_api_build_info{version="..."} on /metrics.
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w -X github.com/LazyGeniusMan/home-ops/projects/apprise-go-api/internal/version.Version=${VERSION}" -o /out/apprise-go-api ./cmd/apprise-go-api

FROM gcr.io/distroless/base-debian13@sha256:9ef50bca108839d5986e4d84b7f7b2d79024c9293b7c35b162c6c55485bd5868
COPY --from=build /out/apprise-go-api /apprise-go-api
# Non-root runtime (65532). No volumes: attachments are request-scoped only.
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/apprise-go-api"]
