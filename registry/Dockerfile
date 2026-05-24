FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.work go.work.sum ./
COPY proto/ proto/
COPY registry/ registry/
RUN CGO_ENABLED=0 go build -trimpath -o /registry github.com/secwager/secwager/registry/cmd/registry

FROM gcr.io/distroless/static-debian12
COPY --from=builder /registry /registry
COPY --from=builder /src/registry/db/migrations /db/migrations
ENTRYPOINT ["/registry"]
