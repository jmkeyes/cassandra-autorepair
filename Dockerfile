# syntax=docker/dockerfile:1

ARG GOLANG_VERSION=1.23

# Build container should always be on the native build platform.
FROM --platform=$BUILDPLATFORM golang:${GOLANG_VERSION} AS builder

WORKDIR /src

ARG TARGETOS TARGETARCH

# Mount the source code into the build container and build the binary.
RUN --mount=type=bind,target=. \
    --mount=type=cache,target=/go/pkg \
    --mount=type=cache,target=/root/.cache/go-build \
    GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /go/bin/cassandra-autorepair

FROM scratch

COPY --from=builder /go/bin/cassandra-autorepair /usr/bin/cassandra-autorepair

# We have a single binary container so just set the entrypoint.
ENTRYPOINT ["/usr/bin/cassandra-autorepair"]
