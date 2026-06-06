ARG GO_VERSION
FROM golang:${GO_VERSION}-alpine AS builder
ARG VERSION=dev
ARG REVISION=dev
WORKDIR /src
RUN apk add --no-cache upx && \
    echo 'nobody:x:65534:65534:Nobody:/:' > /tmp/passwd
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build \
    -ldflags "-s -w -X main.Version=${VERSION} -X main.Gitsha=${REVISION}" \
    -trimpath -o /out/external-dns-unifi-webhook ./cmd/external-dns-unifi-webhook
RUN upx --best --lzma /out/external-dns-unifi-webhook

FROM scratch
COPY --from=builder /tmp/passwd /etc/passwd
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /out/external-dns-unifi-webhook /external-dns-unifi-webhook
USER 65534
EXPOSE 8888/tcp
ENTRYPOINT ["/external-dns-unifi-webhook"]
