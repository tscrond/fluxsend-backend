# syntax=docker/dockerfile:1
#
# Open-source MinIO server.
#
# MinIO discontinued its community distribution in October 2025: the GitHub
# repository is archived, the `minio/minio` Docker Hub image was removed, and
# the dl.min.io binary CDN now returns 410 Gone. The only image MinIO still
# publishes is the licensed AIStor build (quay.io/minio/aistor/minio), which
# refuses to serve requests without an installed licence.
#
# This image repackages the last AGPLv3 community release straight from the
# official GitHub release assets, verified against the published checksum, so
# the stack keeps working without any MinIO licence.
#
# Upstream: https://github.com/minio/minio (archived, AGPL-3.0)

FROM alpine:3.22

ARG TARGETARCH=amd64
ARG MINIO_RELEASE=RELEASE.2025-09-07T16-13-09Z

RUN apk add --no-cache ca-certificates curl && \
    curl -fsSLo "/tmp/minio.${MINIO_RELEASE}" \
      "https://github.com/minio/minio/releases/download/${MINIO_RELEASE}/minio.linux-${TARGETARCH}.${MINIO_RELEASE}" && \
    curl -fsSLo /tmp/minio.sha256sum \
      "https://github.com/minio/minio/releases/download/${MINIO_RELEASE}/minio.linux-${TARGETARCH}.${MINIO_RELEASE}.sha256sum" && \
    cd /tmp && sha256sum -c minio.sha256sum && \
    install -m 0755 "/tmp/minio.${MINIO_RELEASE}" /usr/local/bin/minio && \
    rm -f /tmp/minio.*

VOLUME ["/data"]

EXPOSE 9000 9001

ENTRYPOINT ["/usr/local/bin/minio"]
CMD ["server", "/data"]
