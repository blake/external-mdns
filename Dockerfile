FROM --platform=$BUILDPLATFORM golang:1.16 as build
LABEL maintainer="Blake Covarrubias <blake@covarrubi.as>" \
      org.opencontainers.image.authors="Blake Covarrubias <blake@covarrubi.as>" \
      org.opencontainers.image.description="Advertises records for Kubernetes resources over multicast DNS." \
      org.opencontainers.image.licenses="Apache-2.0" \
      org.opencontainers.image.source="git@github.com:blake/external-mdns" \
      org.opencontainers.image.title="external-mdns" \
      org.opencontainers.image.url="https://github.com/blake/external-mdns"

ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT

ADD . /go/src/github.com/blake/external-mdns
WORKDIR /go/src/github.com/blake/external-mdns

RUN mkdir -p /release/etc &&\
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} GOARM=$(echo ${TARGETVARIANT} | cut -c2) \
    go build \
    -ldflags="-s -w" \
    -o /etc/release/external-mdns . &&\
    echo nobody:x:65534:65534:nobody:/nonexistent:/usr/sbin/nologin > /release/etc/passwd


FROM scratch
COPY --from=build /go/src/github.com/blake/external-mdns/external-mdns /external-mdns
USER nobody
ENTRYPOINT ["/external-mdns"]
