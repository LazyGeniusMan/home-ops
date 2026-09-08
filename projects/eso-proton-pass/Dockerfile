# syntax=docker/dockerfile:1
# eso-proton-pass: ESO webhook provider for Proton Pass (pull-only).
# Multi-stage build: pinned golang toolchain -> pinned distroless runtime.
# Runtime has no shell; the service runs as the distroless non-root user.
ARG PASS_CLI_VERSION=2.3.3

FROM golang:1.26.7@sha256:e30143be198ab04cf7ba25fba83ab3a692ca584c994aad0bf131fa0eb32dd8c1 AS build
ARG TARGETARCH
ARG PASS_CLI_VERSION
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/eso-proton-pass .
# Download the pass-cli binary for the target architecture with pinned
# per-arch SHA-256 checksums (releases page: github.com/protonpass/pass-cli).
RUN set -eux -o pipefail; \
    case "${TARGETARCH:-amd64}" in \
      amd64) _arch=x86_64; _sha256=b5b49a8b3fd0af8830c0c1979f28ea0c90ccece73f59023a8bca8245d4b68da9 ;; \
      arm64) _arch=aarch64; _sha256=9c3e85e10d3bb631ffe377f063d996b9cc9a545d30971bcedf5910e16d03542b ;; \
      *) echo "unsupported TARGETARCH=${TARGETARCH}"; exit 1 ;; \
    esac; \
    curl -fsSL -o /out/pass-cli \
      "https://github.com/protonpass/pass-cli/releases/download/${PASS_CLI_VERSION}/pass-cli-linux-${_arch}"; \
    printf '%s  /out/pass-cli\n' "${_sha256}" | sha256sum -c -; \
    chmod 0755 /out/pass-cli

FROM gcr.io/distroless/base-debian13@sha256:9ef50bca108839d5986e4d84b7f7b2d79024c9293b7c35b162c6c55485bd5868
COPY --from=build /out/eso-proton-pass /eso-proton-pass
COPY --from=build /out/pass-cli /usr/local/bin/pass-cli
# Run as the distroless non-root user (65532), never as root.
USER nonroot:nonroot
EXPOSE 8080
# Telemetry off, container-safe key storage; explicit pass-cli path so the
# service does not depend on runtime PATH contents.
ENV PASS_CLI_BIN=/usr/local/bin/pass-cli \
    PROTON_PASS_DISABLE_TELEMETRY=1 \
    PASS_LOG_LEVEL=off \
    PROTON_PASS_KEY_PROVIDER=fs
ENTRYPOINT ["/eso-proton-pass"]
