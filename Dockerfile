FROM golang:1.16
LABEL maintainer="Blake Covarrubias <blake@covarrubi.as>" \
      org.opencontainers.image.authors="Blake Covarrubias <blake@covarrubi.as>" \
      org.opencontainers.image.description="Advertises records for Kubernetes resources over multicast DNS." \
      org.opencontainers.image.licenses="Apache-2.0" \
      org.opencontainers.image.source="git@github.com:blake/external-mdns" \
      org.opencontainers.image.title="external-mdns" \
      org.opencontainers.image.url="https://github.com/blake/external-mdns"

ARG TARGETOS
ARG TARGETARCH

ADD . /go/src/github.com/blake/external-mdns
WORKDIR /go/src/github.com/blake/external-mdns

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build \
    -ldflags="-s -w" \
    -o external-mdns .

FROM scratch
COPY --from=0 /go/src/github.com/blake/external-mdns/external-mdns /external-mdns
ENTRYPOINT ["/external-mdns"]
