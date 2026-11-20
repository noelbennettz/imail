# syntax=docker/dockerfile:1
# iMail - internal use build.
ARG BUILDPLATFORM
# Base images are pinned by digest (the multi-arch manifest list) so a rebuild
# cannot silently pick up a different image. To update, run
#   docker buildx imagetools inspect <image>:<tag>
# and replace the digest.
FROM --platform=$BUILDPLATFORM node:26-alpine@sha256:6bd9ab43c0ec0190ca7b2d193906cb8b7edf6e7f4afa43ddeccd6452710bffb5 AS febuild
WORKDIR /work

COPY fe .
RUN yarn install --frozen-lockfile --network-timeout 600000 && yarn build


FROM --platform=$BUILDPLATFORM golang:1.27-alpine@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280 AS serverbuild
ARG VERSION=internal
ARG TARGETOS
ARG TARGETARCH
WORKDIR /work
ENV CGO_ENABLED=0
ENV GOFLAGS=-mod=vendor
COPY . .
COPY --from=febuild /work/dist /work/server/listen/http_server/dist
RUN cd /work/server && GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -mod=vendor -trimpath -ldflags "-s -w -X 'main.version=${VERSION}' -X 'main.goVersion=$(go version)' -X 'main.gitHash=internal' -X 'main.buildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)'" \
    -o imail main.go


FROM alpine:3.24@sha256:f59abe656b692d901e184be3445e084786602108599d2a51d229c0f5a48c71bf

LABEL org.opencontainers.image.title="iMail" \
      org.opencontainers.image.description="Self-hosted mail server" \
      org.opencontainers.image.licenses="AGPL-3.0"

WORKDIR /work

RUN apk add --no-cache tzdata ca-certificates wget openssl \
    && mkdir -p /work/config \
    && rm -rf /var/cache/apk/* /tmp/* /var/tmp/*

COPY --from=serverbuild /work/server/imail ./imail

# AGPL-3.0: whoever receives the image gets the licence text and the third-party
# notices with the binary, not just a link in the README.
COPY LICENSE THIRD_PARTY_NOTICES.md /usr/share/doc/imail/

EXPOSE 25 80 110 443 465 587 993 995

HEALTHCHECK --interval=30s --timeout=5s --start-period=45s --retries=3 \
    CMD wget -qO- "http://127.0.0.1:${HTTP_PORT:-80}/api/ping" >/dev/null 2>&1 || wget -qO- "http://[::1]:${HTTP_PORT:-80}/api/ping" >/dev/null 2>&1 || exit 1

CMD ["/work/imail"]
