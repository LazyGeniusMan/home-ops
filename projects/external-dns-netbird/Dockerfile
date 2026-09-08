# syntax=docker/dockerfile:1
# ExternalDNS NetBird webhook provider.
# Multi-stage build: golang toolchain -> distroless runtime, non-root.

FROM golang:1.26.7@sha256:e30143be198ab04cf7ba25fba83ab3a692ca584c994aad0bf131fa0eb32dd8c1 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/external-dns-netbird ./cmd/external-dns-netbird

FROM gcr.io/distroless/base-debian13@sha256:9ef50bca108839d5986e4d84b7f7b2d79024c9293b7c35b162c6c55485bd5868
COPY --from=build /out/external-dns-netbird /external-dns-netbird
# Run as the distroless nonroot user (65532), never as root.
USER nonroot:nonroot
# Webhook API port (localhost sidecar). Metrics/health port 8080 is separate.
EXPOSE 8888
ENTRYPOINT ["/external-dns-netbird"]
