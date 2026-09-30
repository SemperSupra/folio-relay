# syntax=docker/dockerfile:1.7

ARG GO_BASE=golang:1.27.1-bookworm@sha256:69a7b9788769bec032d238959b61854e9ae87f57be9029ec04e9885fabf99195

FROM ${GO_BASE} AS build
WORKDIR /src
COPY go.mod ./
COPY api ./api
COPY cmd/foliorelayd ./cmd/foliorelayd
COPY cmd/foliorelay-probe ./cmd/foliorelay-probe
COPY internal/control ./internal/control
COPY internal/inbox ./internal/inbox
COPY internal/printer ./internal/printer
COPY internal/security ./internal/security
COPY internal/state ./internal/state
COPY internal/webui ./internal/webui
RUN CGO_ENABLED=0 go test ./cmd/foliorelayd ./cmd/foliorelay-probe ./internal/control ./internal/inbox ./internal/printer ./internal/security ./internal/state  && CGO_ENABLED=0 go build -trimpath -ldflags='-s -w -buildid=' -o /out/foliorelayd ./cmd/foliorelayd  && CGO_ENABLED=0 go build -trimpath -ldflags='-s -w -buildid=' -o /out/foliorelay-probe ./cmd/foliorelay-probe

FROM scratch
ARG GO_BASE
LABEL org.opencontainers.image.source="https://github.com/SemperSupra/folio-relay"       org.opencontainers.image.title="FolioRelay Control Plane"       io.sempersupra.base.go="${GO_BASE}"
COPY --from=build /out/foliorelayd /usr/local/bin/foliorelayd
COPY --from=build /out/foliorelay-probe /usr/local/bin/foliorelay-probe
USER 10001:10001
VOLUME ["/var/lib/foliorelay"]
EXPOSE 18080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 CMD ["/usr/local/bin/foliorelay-probe"]
ENTRYPOINT ["/usr/local/bin/foliorelayd"]
