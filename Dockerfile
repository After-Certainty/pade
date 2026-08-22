# Multi-stage build for the pade-broker runtime image only (not the pade CLI).
# No secrets, policy, or bindings are baked into the image.

FROM golang:1.26-bookworm AS build
ARG VERSION=dev
ARG VCS_REF=unknown
ARG BUILD_TIME=
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags "-s -w \
  -X github.com/ksteffe/pade/internal/version.Version=${VERSION} \
  -X github.com/ksteffe/pade/internal/version.Commit=${VCS_REF} \
  -X github.com/ksteffe/pade/internal/version.BuildTime=${BUILD_TIME}" \
  -o /out/pade-broker ./cmd/pade-broker

# static-debian12 includes CA certificates for outbound HTTPS (JWKS, Keeper).
# nonroot user (uid 65532). No shell/curl — platforms probe application /healthz.
# Docker HEALTHCHECK omitted: distroless has no HTTP client; Cloud Run/K8s probe /healthz.
FROM gcr.io/distroless/static-debian12:nonroot
ARG VERSION=dev
ARG VCS_REF=unknown
ARG IMAGE_SOURCE=https://github.com/After-Certainty/pade
LABEL org.opencontainers.image.title="pade-broker" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${VCS_REF}" \
      org.opencontainers.image.source="${IMAGE_SOURCE}"
COPY --from=build /out/pade-broker /pade-broker
# Container platforms (Cloud Run, etc.) typically inject PORT; default aids local docker run.
ENV PORT=8080
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/pade-broker"]
