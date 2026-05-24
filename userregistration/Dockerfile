FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.work go.work.sum ./
COPY proto/ proto/
COPY userregistration/ userregistration/
RUN CGO_ENABLED=0 go build -trimpath -o /userregistration github.com/secwager/secwager/userregistration/cmd/userregistration

FROM gcr.io/distroless/static-debian12
COPY --from=builder /userregistration /userregistration
COPY --from=builder /src/userregistration/db/migrations /db/migrations
ENTRYPOINT ["/userregistration"]
