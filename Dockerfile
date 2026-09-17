# The build stage runs on the build machine's own architecture and
# cross-compiles, so the arm64 image needs no QEMU to build - Go does it
# natively with CGO off. The result is one static binary with the dashboard,
# the world map and the region and network tables embedded.
FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS build
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
WORKDIR /src
COPY go.mod ./
COPY *.go ./
COPY geo ./geo
COPY asn ./asn
COPY web ./web
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w -X main.Version=${VERSION}" -o /out/peermap .

# Nothing but the binary: no shell, no package manager, no libc. It talks
# plain HTTP to Bitcoin Core on the local Docker network, so it needs no CA
# certificates either.
FROM scratch
COPY --from=build /out/peermap /peermap
USER 1000:1000
EXPOSE 8789
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 CMD ["/peermap", "healthcheck"]
ENTRYPOINT ["/peermap"]
