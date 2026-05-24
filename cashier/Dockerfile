FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.work go.work.sum ./
COPY proto/ proto/
COPY cashier/ cashier/
RUN CGO_ENABLED=0 go build -trimpath -o /cashier github.com/secwager/secwager/cashier/cmd/cashier

FROM gcr.io/distroless/static-debian12
COPY --from=builder /cashier /cashier
COPY --from=builder /src/cashier/db/migrations /db/migrations
ENTRYPOINT ["/cashier"]
