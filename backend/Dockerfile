# syntax=docker/dockerfile:1

# Match the go directive in go.mod.
ARG GO_VERSION=1.27

FROM golang:${GO_VERSION}-alpine AS build
WORKDIR /src
# Resolve modules in their own layer so a source change does not re-download them.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

# Static binary on a minimal, non-root base image (no shell, no package manager).
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/api /api
ENV HTTP_ADDR=:8080
EXPOSE 8080
USER 65532:65532
HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 CMD ["/api", "-healthcheck"]
ENTRYPOINT ["/api"]
