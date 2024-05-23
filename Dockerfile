FROM golang:1.22-alpine as builder
WORKDIR /build
COPY go.mod go.sum /build/
RUN go mod download
COPY . .
RUN go build -o /external-dns-unifi-webhook

FROM gcr.io/distroless/static-debian12:nonroot
USER 8675:8675
COPY --from=builder --chmod=555 /external-dns-unifi-webhook /usr/local/bin/external-dns-unifi-webhook
EXPOSE 8888/tcp
ENTRYPOINT ["/usr/local/bin/external-dns-unifi-webhook"]
