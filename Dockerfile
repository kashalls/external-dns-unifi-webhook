FROM golang:1.23-alpine as builder
ARG PKG=github.com/kashalls/external-dns-unifi-webhook
ARG VERSION=dev
ARG REVISION=dev
WORKDIR /build
COPY . .
RUN go build -ldflags "-s -w -X main.Version=${VERSION} -X main.Gitsha=${REVISION}" ./cmd/webhook

FROM gcr.io/distroless/static-debian12:nonroot
USER 8675:8675
COPY --from=builder --chmod=555 /build/webhook /external-dns-unifi-webhook
EXPOSE 8888/tcp
ENTRYPOINT ["/external-dns-unifi-webhook"]
