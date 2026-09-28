# syntax=docker/dockerfile:1.7

ARG GO_BASE=golang:1.27.1-bookworm@sha256:69a7b9788769bec032d238959b61854e9ae87f57be9029ec04e9885fabf99195
ARG UBUNTU_BASE=ubuntu:24.04@sha256:008173c23f95b170204355c12626cb5a965d779a7e1283b09e9cffbb1bf33ca3
ARG UBUNTU_SNAPSHOT=20260928T000000Z

FROM ${GO_BASE} AS go-build
WORKDIR /src
COPY go.mod ./
COPY cmd/foliorelay-ipp-ingest ./cmd/foliorelay-ipp-ingest
COPY cmd/foliorelay-state-fixture ./cmd/foliorelay-state-fixture
COPY internal/state ./internal/state
COPY internal/security ./internal/security
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/foliorelay-ipp-ingest ./cmd/foliorelay-ipp-ingest \
 && CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/foliorelay-state-fixture ./cmd/foliorelay-state-fixture

FROM ${UBUNTU_BASE} AS cups-build
ARG UBUNTU_SNAPSHOT
ARG CUPS_VERSION=v2.4.19
ARG CUPS_COMMIT=6ba0487abb05afc93d639f37676add8fd65d3756
COPY --from=go-build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
ENV DEBIAN_FRONTEND=noninteractive
RUN printf 'APT::Snapshot "%s";\n' "${UBUNTU_SNAPSHOT}" > /etc/apt/apt.conf.d/50snapshot \
 && apt-get update \
 && apt-get install -y --no-install-recommends \
      autoconf build-essential ca-certificates git libssl-dev pkg-config zlib1g-dev \
 && rm -rf /var/lib/apt/lists/*
WORKDIR /src
RUN git init cups \
 && cd cups \
 && git remote add origin https://github.com/OpenPrinting/cups.git \
 && git fetch --depth 1 origin "${CUPS_COMMIT}" \
 && git checkout --detach FETCH_HEAD \
 && test "$(git rev-parse HEAD)" = "${CUPS_COMMIT}"
WORKDIR /src/cups
RUN ./configure \
      --prefix=/usr \
      --sysconfdir=/etc \
      --localstatedir=/var \
      --with-tls=openssl \
      --with-dnssd=no \
      --disable-pam \
      --disable-gssapi \
      --disable-libusb \
      --disable-webif \
      --disable-browsing \
      --disable-default-shared \
      --disable-raw-printing \
      --with-languages=none \
      --with-printcap=no \
      --with-max-copies=1000 \
 && make -j"$(nproc)" \
 && mkdir -p /out \
 && make BUILDROOT=/out install \
 && install -Dm0555 /out/usr/sbin/cupsd /runtime/usr/sbin/cupsd \
 && install -Dm0555 /out/usr/lib/cups/daemon/cups-exec /runtime/usr/lib/cups/daemon/cups-exec \
 && install -Dm0555 /out/usr/lib/cups/filter/gziptoany /runtime/usr/lib/cups/filter/gziptoany \
 && install -Dm0555 /src/cups/cups/libcups.so.2 /runtime/usr/lib/foliorelay-cups/libcups.so.2 \
 && mkdir -p /runtime/usr/share/cups/mime \
 && cp -a /out/usr/share/cups/mime/. /runtime/usr/share/cups/mime/

FROM ${UBUNTU_BASE} AS state
RUN groupadd -g 10001 foliorelay \
 && useradd --no-create-home --uid 10001 --gid 10001 --shell /usr/sbin/nologin foliorelay \
 && mkdir -p /state \
 && chown -R 10001:10001 /state
COPY --from=go-build /out/foliorelay-state-fixture /usr/local/bin/foliorelay-state-fixture
USER 10001:10001
VOLUME ["/state"]
EXPOSE 18080
ENTRYPOINT ["/usr/local/bin/foliorelay-state-fixture"]
CMD ["-listen","0.0.0.0:18080","-journal","/state/journal.frj"]

FROM ${UBUNTU_BASE} AS cups
ARG UBUNTU_SNAPSHOT
ARG CUPS_VERSION=v2.4.19
ARG CUPS_COMMIT=6ba0487abb05afc93d639f37676add8fd65d3756
ARG UBUNTU_BASE
ARG GO_BASE
COPY --from=go-build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
ENV DEBIAN_FRONTEND=noninteractive
RUN printf 'APT::Snapshot "%s";\n' "${UBUNTU_SNAPSHOT}" > /etc/apt/apt.conf.d/50snapshot \
 && apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates libssl3 zlib1g \
 && rm -rf /var/lib/apt/lists/* \
 && groupadd -g 10001 foliorelay \
 && useradd --no-create-home --uid 10001 --gid 10001 --shell /usr/sbin/nologin foliorelay
LABEL org.opencontainers.image.source="https://github.com/OpenPrinting/cups" \
      org.opencontainers.image.version="${CUPS_VERSION}" \
      org.opencontainers.image.revision="${CUPS_COMMIT}" \
      io.sempersupra.ubuntu-snapshot="${UBUNTU_SNAPSHOT}" \
      io.sempersupra.base.ubuntu="${UBUNTU_BASE}" \
      io.sempersupra.base.go="${GO_BASE}"
COPY --from=cups-build /runtime/usr/ /usr/
COPY --from=go-build /out/foliorelay-ipp-ingest /usr/local/bin/foliorelay-ipp-ingest
COPY experiments/print-substrate/cups-backend.sh /usr/lib/cups/backend/foliorelay
COPY experiments/print-substrate/cups-foliorelay.ppd /etc/cups/ppd/FolioRelay.ppd
COPY deploy/engineering-cups/cups-files.conf /etc/cups/cups-files.conf
COPY deploy/engineering-cups/cupsd.conf /etc/cups/cupsd.conf
COPY deploy/engineering-cups/printers.conf /etc/cups/printers.conf
COPY deploy/engineering-cups/start-cups.sh /usr/local/bin/start-foliorelay-cups
RUN chmod 0555 /usr/sbin/cupsd /usr/lib/cups/daemon/cups-exec /usr/lib/cups/filter/gziptoany /usr/lib/cups/backend/foliorelay \
 && chmod 0555 /usr/local/bin/start-foliorelay-cups \
 && mkdir -p /var/lib/cups/ssl /var/cache/cups /var/spool/cups/tmp /var/log/cups /var/lib/foliorelay-artifacts \
 && chown -R 10001:10001 /etc/cups /var/lib/cups /var/cache/cups /var/spool/cups /var/log/cups /var/lib/foliorelay-artifacts
ENV LD_LIBRARY_PATH=/usr/lib/foliorelay-cups:/usr/lib \
    FOLIORELAY_INGEST_BIN=/usr/local/bin/foliorelay-ipp-ingest \
    FOLIORELAY_STATE_URL=http://state:18080 \
    FOLIORELAY_INGRESS_STORE=/var/lib/foliorelay-artifacts \
    FOLIORELAY_MAX_INGRESS_BYTES=104857600
USER 10001:10001
VOLUME ["/var/lib/cups","/var/spool/cups","/var/lib/foliorelay-artifacts"]
EXPOSE 8634 8635
ENTRYPOINT ["/usr/local/bin/start-foliorelay-cups"]
