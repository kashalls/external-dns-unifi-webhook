FROM golang:1.22-alpine as base

FROM base as builder
# Work directory
WORKDIR /build

# Installing dependencies
COPY go.mod go.sum /build/

RUN go mod download

# Copying all the files
COPY . .

# Build our application
RUN go build -o /external-dns-unifi-webhook

FROM alpine:latest

COPY --from=builder --chown=root:root external-dns-unifi-webhook /bin/

EXPOSE 8888

# Drop to unprivileged user to run
USER nobody
CMD ["/bin/external-dns-unifi-webhook"]
