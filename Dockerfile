# syntax=docker/dockerfile:1
# iMail - internal use build.
ARG BUILDPLATFORM
FROM --platform=$BUILDPLATFORM node:22-alpine AS febuild
WORKDIR /work

COPY fe .
RUN yarn install --frozen-lockfile --network-timeout 600000 && yarn build


FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS serverbuild
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


FROM alpine:3.21

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
    CMD wget -qO- "http://127.0.0.1:${HTTP_PORT:-80}/api/ping" >/dev/null 2>&1 || exit 1

CMD ["/work/imail"]
