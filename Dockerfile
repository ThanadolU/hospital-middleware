# syntax=docker/dockerfile:1

# One image carries all three binaries. They share a module, a dependency set
# and a build cache, so building them separately would triple the work to
# produce three files totalling a few megabytes.

FROM golang:1.25-alpine AS build

WORKDIR /src

# Dependencies first: this layer is rebuilt only when go.mod or go.sum change,
# not on every source edit.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .

# CGO_ENABLED=0 produces a static binary, which is what lets the runtime stage
# be a bare alpine with no Go toolchain and no libc surprises.
# -trimpath keeps build-host paths out of panics; -s -w drops the symbol table
# and DWARF, roughly halving the binary.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags='-s -w' -o /out/ ./cmd/...

FROM alpine:3.21 AS runtime

# wget comes from busybox and is what the compose healthchecks call. A
# distroless base would be smaller, but it has no shell and no HTTP client, so
# the healthcheck that proves the service can reach its database could not run.
RUN apk add --no-cache ca-certificates tzdata

# A numeric, unprivileged user. Nothing in the image is owned by it, so a
# compromised process cannot rewrite its own binary.
RUN adduser -D -u 10001 app
USER 10001:10001

COPY --from=build /out/ /usr/local/bin/

# The port cmd/api actually listens on. v1 exposed a different port from the one
# it bound, so `docker run -P` published nothing useful.
EXPOSE 8000

ENTRYPOINT ["/usr/local/bin/api"]
