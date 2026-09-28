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

FROM ${UBUNTU_BASE} AS pappl-build
ARG UBUNTU_SNAPSHOT
ARG PAPPL_VERSION=v1.4.11
ARG PAPPL_COMMIT=ad86a0a8473f0f83233a374226561181570d4f81
ARG CUPS_VERSION=v2.4.19
ARG CUPS_COMMIT=6ba0487abb05afc93d639f37676add8fd65d3756
COPY --from=go-build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
ENV DEBIAN_FRONTEND=noninteractive
RUN printf 'APT::Snapshot "%s";\n' "${UBUNTU_SNAPSHOT}" > /etc/apt/apt.conf.d/50snapshot \
 && apt-get update \
 && apt-get install -y --no-install-recommends \
      autoconf build-essential ca-certificates git pkg-config \
      libssl-dev zlib1g-dev libpng-dev libjpeg-dev \
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
      --libdir=/usr/lib/foliorelay-cups \
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
 && make install
ENV LD_LIBRARY_PATH=/usr/lib/foliorelay-cups
WORKDIR /src
RUN git init pappl \
 && cd pappl \
 && git remote add origin https://github.com/michaelrsweet/pappl.git \
 && git fetch --depth 1 origin "${PAPPL_COMMIT}" \
 && git checkout --detach FETCH_HEAD \
 && test "$(git rev-parse HEAD)" = "${PAPPL_COMMIT}"
WORKDIR /src/pappl
RUN ./configure \
      --with-dnssd=no \
      --with-tls=openssl \
      --disable-libpam \
      --disable-libusb \
      --disable-shared \
 && make -j"$(nproc)"
WORKDIR /src
COPY experiments/print-substrate/pappl-ingress.c /src/pappl-ingress.c
RUN mkdir -p /out /runtime \
 && cc -D_GNU_SOURCE \
      -I/src/pappl -I/src/pappl/pappl \
      $(cups-config --cflags) \
      -L/usr/lib/foliorelay-cups \
      -o /out/foliorelay-pappl-ingress \
      /src/pappl-ingress.c \
      /src/pappl/pappl/libpappl.a \
      $(cups-config --image --libs) \
      $(pkg-config --libs openssl libjpeg libpng zlib) \
      -lpthread -lm \
 && strip /out/foliorelay-pappl-ingress \
 && ldd /out/foliorelay-pappl-ingress > /out/foliorelay-pappl-ingress.ldd \
 && install -Dm0555 /usr/lib/foliorelay-cups/libcups.so.2 \
      /runtime/usr/lib/foliorelay-cups/libcups.so.2

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

FROM ${UBUNTU_BASE} AS pappl
ARG UBUNTU_SNAPSHOT
ARG PAPPL_VERSION=v1.4.11
ARG PAPPL_COMMIT=ad86a0a8473f0f83233a374226561181570d4f81
ARG CUPS_VERSION=v2.4.19
ARG CUPS_COMMIT=6ba0487abb05afc93d639f37676add8fd65d3756
ARG UBUNTU_BASE
ARG GO_BASE
COPY --from=go-build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
ENV DEBIAN_FRONTEND=noninteractive
RUN printf 'APT::Snapshot "%s";\n' "${UBUNTU_SNAPSHOT}" > /etc/apt/apt.conf.d/50snapshot \
 && apt-get update \
 && apt-get install -y --no-install-recommends \
      ca-certificates libjpeg-turbo8 libpng16-16t64 libssl3 zlib1g \
 && rm -rf /var/lib/apt/lists/* \
 && groupadd -g 10001 foliorelay \
 && useradd --no-create-home --uid 10001 --gid 10001 --shell /usr/sbin/nologin foliorelay \
 && mkdir -p /var/lib/pappl /var/lib/foliorelay-artifacts \
 && chown -R 10001:10001 /var/lib/pappl /var/lib/foliorelay-artifacts
LABEL org.opencontainers.image.source="https://github.com/michaelrsweet/pappl" \
      org.opencontainers.image.version="${PAPPL_VERSION}" \
      org.opencontainers.image.revision="${PAPPL_COMMIT}" \
      io.sempersupra.cups.version="${CUPS_VERSION}" \
      io.sempersupra.cups.revision="${CUPS_COMMIT}" \
      io.sempersupra.ubuntu-snapshot="${UBUNTU_SNAPSHOT}" \
      io.sempersupra.base.ubuntu="${UBUNTU_BASE}" \
      io.sempersupra.base.go="${GO_BASE}"
COPY --from=pappl-build /runtime/usr/ /usr/
COPY --from=pappl-build --chmod=0555 /out/foliorelay-pappl-ingress /usr/local/bin/foliorelay-pappl-ingress
COPY --from=pappl-build /out/foliorelay-pappl-ingress.ldd /usr/share/foliorelay/pappl-runtime.ldd
COPY --from=go-build --chmod=0555 /out/foliorelay-ipp-ingest /usr/local/bin/foliorelay-ipp-ingest
COPY --chmod=0555 deploy/engineering-pappl/start-pappl.sh /usr/local/bin/start-foliorelay-pappl
ENV LD_LIBRARY_PATH=/usr/lib/foliorelay-cups:/usr/lib \
    FOLIORELAY_INGEST_BIN=/usr/local/bin/foliorelay-ipp-ingest \
    FOLIORELAY_STATE_URL=http://state:18080 \
    FOLIORELAY_INGRESS_STORE=/var/lib/foliorelay-artifacts \
    FOLIORELAY_MAX_INGRESS_BYTES=104857600 \
    FOLIORELAY_PAPPL_SPOOL=/var/lib/pappl \
    FOLIORELAY_PAPPL_PORT=8633 \
    HOME=/var/lib/pappl \
    TMPDIR=/tmp
USER 10001:10001
VOLUME ["/var/lib/pappl","/var/lib/foliorelay-artifacts"]
EXPOSE 8633
ENTRYPOINT ["/usr/local/bin/start-foliorelay-pappl"]
