ARG ALPINE_VERSION=3.24
FROM alpine:${ALPINE_VERSION}
ARG ALPINE_VERSION

LABEL dev.govard.image.name="dnsmasq"
LABEL dev.govard.dnsmasq.version="${ALPINE_VERSION}"

RUN apk add --no-cache dnsmasq
EXPOSE 53/tcp 53/udp
ENTRYPOINT ["dnsmasq", "-k"]
