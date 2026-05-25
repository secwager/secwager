FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY proto/ proto/
COPY registry/ registry/
RUN cd registry && CGO_ENABLED=0 GOWORK=off go build -trimpath -o /registry ./cmd/registry

FROM gcr.io/distroless/static-debian12
COPY --from=builder /registry /registry
COPY --from=builder /src/registry/db/migrations /db/migrations
ENTRYPOINT ["/registry"]
