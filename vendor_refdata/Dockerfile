FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY proto/ proto/
COPY vendor_refdata/ vendor_refdata/
RUN cd vendor_refdata && CGO_ENABLED=0 GOWORK=off go build -trimpath -o /vendor_refdata ./cmd/vendor_refdata

FROM gcr.io/distroless/static-debian12
COPY --from=builder /vendor_refdata /vendor_refdata
COPY registry/db/migrations /db/migrations
ENTRYPOINT ["/vendor_refdata"]
