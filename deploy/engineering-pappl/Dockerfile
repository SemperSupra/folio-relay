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

FROM ubuntu:24.04 AS pappl-build
ARG PAPPL_VERSION=v1.4.11
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update \
 && apt-get install -y --no-install-recommends \
      autoconf build-essential ca-certificates git pkg-config \
      libssl-dev zlib1g-dev libcups2-dev libpng-dev libjpeg-dev \
 && rm -rf /var/lib/apt/lists/*
WORKDIR /src
RUN git clone --depth 1 --branch "${PAPPL_VERSION}" https://github.com/michaelrsweet/pappl.git pappl
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
      -o /out/foliorelay-pappl-ingress \
      /src/pappl-ingress.c \
      /src/pappl/pappl/libpappl.a \
      $(cups-config --image --libs) \
      $(pkg-config --libs openssl libjpeg libpng zlib) \
      -lpthread -lm \
 && strip /out/foliorelay-pappl-ingress \
 && ldd /out/foliorelay-pappl-ingress > /out/foliorelay-pappl-ingress.ldd \
 && { \
      awk '/=> \/[^ ]+/ {print $3} /^[[:space:]]*\/[^ ]+/ {print $1}' /out/foliorelay-pappl-ingress.ldd; \
    } | sort -u | while read -r lib; do \
      test -n "$lib" && cp --parents "$lib" /runtime; \
    done

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

FROM ubuntu:24.04 AS pappl
RUN groupadd -g 10001 foliorelay \
 && useradd --no-create-home --uid 10001 --gid 10001 --shell /usr/sbin/nologin foliorelay \
 && mkdir -p /var/lib/pappl /var/lib/foliorelay-artifacts \
 && chown -R 10001:10001 /var/lib/pappl /var/lib/foliorelay-artifacts
COPY --from=pappl-build /runtime/ /
COPY --from=pappl-build /out/foliorelay-pappl-ingress /usr/local/bin/foliorelay-pappl-ingress
COPY --from=pappl-build /out/foliorelay-pappl-ingress.ldd /usr/share/foliorelay/pappl-runtime.ldd
COPY --from=go-build /out/foliorelay-ipp-ingest /usr/local/bin/foliorelay-ipp-ingest
COPY deploy/engineering-pappl/start-pappl.sh /usr/local/bin/start-foliorelay-pappl
RUN chmod 0555 /usr/local/bin/foliorelay-pappl-ingress \
                  /usr/local/bin/foliorelay-ipp-ingest \
                  /usr/local/bin/start-foliorelay-pappl
ENV FOLIORELAY_INGEST_BIN=/usr/local/bin/foliorelay-ipp-ingest \
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
