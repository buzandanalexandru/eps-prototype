# syntax=docker/dockerfile:1
# One multi-stage build for every Go binary (S2); pick one with --target <binary>.
# Final images are distroless/static: the binary plus CA certificates, no shell, no env files, no keys.
#   docker build --target core --build-arg COMMIT=$(git rev-parse HEAD) -t eps-core services/

FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
ARG COMMIT=dev
ARG BUILT_AT=
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    set -e; \
    pkg=github.com/buzandanalexandru/eps-prototype/services/internal/version; \
    for b in gateway core worker fakepsp simulator; do \
      CGO_ENABLED=0 GOOS=linux go build -trimpath \
        -ldflags "-s -w -X $pkg.Commit=$COMMIT -X $pkg.BuiltAt=$BUILT_AT" \
        -o /out/$b ./cmd/$b; \
    done

FROM gcr.io/distroless/static-debian12:nonroot AS base
USER nonroot:nonroot
EXPOSE 8080

FROM base AS gateway
COPY --from=build /out/gateway /gateway
ENTRYPOINT ["/gateway"]

FROM base AS core
COPY --from=build /out/core /core
ENTRYPOINT ["/core"]

FROM base AS worker
COPY --from=build /out/worker /worker
ENTRYPOINT ["/worker"]

FROM base AS fakepsp
COPY --from=build /out/fakepsp /fakepsp
ENTRYPOINT ["/fakepsp"]

FROM base AS simulator
COPY --from=build /out/simulator /simulator
ENTRYPOINT ["/simulator"]
