FROM golang:1.25-alpine AS builder
ARG PKG=github.com/kashalls/external-dns-unifi-webhook
ARG VERSION=dev
ARG REVISION=dev

RUN echo 'nobody:x:65534:65534:Nobody:/:' > /tmp/passwd && \
    apk add --no-cache upx=5.0.2-r0

WORKDIR /build
COPY . .
RUN CGO_ENABLED=0 go build -ldflags "-s -w -X main.Version=${VERSION} -X main.Gitsha=${REVISION}" ./cmd/webhook && \
    upx --best --lzma webhook

FROM scratch

COPY --from=builder /tmp/passwd /etc/passwd
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder --chmod=555 /build/webhook /external-dns-unifi-webhook

USER nobody
EXPOSE 8888/tcp
ENTRYPOINT ["/external-dns-unifi-webhook"]
