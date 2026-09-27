# syntax=docker/dockerfile:1
# ExternalDNS NetBird webhook provider.

FROM golang:1.26.7@sha256:e30143be198ab04cf7ba25fba83ab3a692ca584c994aad0bf131fa0eb32dd8c1 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w -X github.com/LazyGeniusMan/home-ops/projects/external-dns-netbird/internal/version.Version=${VERSION}" -o /out/external-dns-netbird ./cmd/external-dns-netbird

FROM gcr.io/distroless/base-debian13@sha256:9ef50bca108839d5986e4d84b7f7b2d79024c9293b7c35b162c6c55485bd5868
COPY --from=build /out/external-dns-netbird /external-dns-netbird
# Non-root runtime (65532). Webhook API port; metrics/health use 8080.
USER nonroot:nonroot
EXPOSE 8888
ENTRYPOINT ["/external-dns-netbird"]
