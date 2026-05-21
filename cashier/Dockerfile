FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /cashier ./cmd/cashier

FROM gcr.io/distroless/static-debian12
COPY --from=builder /cashier /cashier
COPY --from=builder /src/db/migrations /db/migrations
ENTRYPOINT ["/cashier"]
