FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY proto/ proto/
COPY userregistration/ userregistration/
RUN cd userregistration && CGO_ENABLED=0 GOWORK=off go build -trimpath -o /userregistration ./cmd/userregistration

FROM gcr.io/distroless/static-debian12
COPY --from=builder /userregistration /userregistration
COPY --from=builder /src/userregistration/db/migrations /db/migrations
ENTRYPOINT ["/userregistration"]
