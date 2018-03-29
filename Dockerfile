FROM golang:1.9 AS builder
WORKDIR /go/src/github.com/pusher/prom-rule-reloader
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/prom-rule-reloader github.com/pusher/prom-rule-reloader

FROM alpine
COPY --from=builder /bin/prom-rule-reloader /bin/prom-rule-reloader

ENTRYPOINT ["/bin/prom-rule-reloader"]
