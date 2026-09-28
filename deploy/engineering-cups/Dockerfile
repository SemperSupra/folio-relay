# syntax=docker/dockerfile:1.7

ARG GO_VERSION=1.27.1

FROM golang:${GO_VERSION}-bookworm AS go-build
WORKDIR /src
COPY go.mod ./
COPY cmd/foliorelay-ipp-ingest ./cmd/foliorelay-ipp-ingest
COPY cmd/foliorelay-state-fixture ./cmd/foliorelay-state-fixture
COPY internal/state ./internal/state
COPY internal/security ./internal/security
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/foliorelay-ipp-ingest ./cmd/foliorelay-ipp-ingest \
 && CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/foliorelay-state-fixture ./cmd/foliorelay-state-fixture

FROM ubuntu:24.04 AS cups-build
ARG CUPS_VERSION=v2.4.19
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update \
 && apt-get install -y --no-install-recommends \
      autoconf build-essential ca-certificates git libssl-dev pkg-config zlib1g-dev \
 && rm -rf /var/lib/apt/lists/*
WORKDIR /src
RUN git clone --depth 1 --branch "${CUPS_VERSION}" https://github.com/OpenPrinting/cups.git cups
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
 && make BUILDROOT=/out install

FROM ubuntu:24.04 AS state
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

FROM ubuntu:24.04 AS cups
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates libssl3 zlib1g \
 && rm -rf /var/lib/apt/lists/* \
 && groupadd -g 10001 foliorelay \
 && useradd --no-create-home --uid 10001 --gid 10001 --shell /usr/sbin/nologin foliorelay
COPY --from=cups-build /out/usr/ /usr/
COPY --from=go-build /out/foliorelay-ipp-ingest /usr/local/bin/foliorelay-ipp-ingest
COPY experiments/print-substrate/cups-backend.sh /usr/lib/cups/backend/foliorelay
COPY experiments/print-substrate/cups-foliorelay.ppd /etc/cups/ppd/FolioRelay.ppd
COPY deploy/engineering-cups/cups-files.conf /etc/cups/cups-files.conf
COPY deploy/engineering-cups/cupsd.conf /etc/cups/cupsd.conf
COPY deploy/engineering-cups/printers.conf /etc/cups/printers.conf
COPY deploy/engineering-cups/start-cups.sh /usr/local/bin/start-foliorelay-cups
RUN chmod 0555 /usr/lib/cups/backend/foliorelay \
 && chmod 0555 /usr/local/bin/start-foliorelay-cups \
 && mkdir -p /var/lib/cups/ssl /var/cache/cups /var/spool/cups/tmp /var/log/cups /var/lib/foliorelay-artifacts \
 && chown -R 10001:10001 /etc/cups /var/lib/cups /var/cache/cups /var/spool/cups /var/log/cups /var/lib/foliorelay-artifacts
ENV FOLIORELAY_INGEST_BIN=/usr/local/bin/foliorelay-ipp-ingest \
    FOLIORELAY_STATE_URL=http://state:18080 \
    FOLIORELAY_INGRESS_STORE=/var/lib/foliorelay-artifacts \
    FOLIORELAY_MAX_INGRESS_BYTES=104857600
USER 10001:10001
VOLUME ["/var/lib/cups","/var/spool/cups","/var/lib/foliorelay-artifacts"]
EXPOSE 8634 8635
ENTRYPOINT ["/usr/local/bin/start-foliorelay-cups"]
