FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY proto/ proto/
COPY cashier/ cashier/
RUN cd cashier && CGO_ENABLED=0 GOWORK=off go build -trimpath -o /cashier ./cmd/cashier

FROM gcr.io/distroless/static-debian12
COPY --from=builder /cashier /cashier
COPY --from=builder /src/cashier/db/migrations /db/migrations
ENTRYPOINT ["/cashier"]
