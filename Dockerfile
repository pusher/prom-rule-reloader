FROM golang:1.10.3 AS builder

RUN curl https://raw.githubusercontent.com/golang/dep/master/install.sh | sh

WORKDIR /go/src/github.com/pusher/prom-rule-reloader

COPY Gopkg.lock Gopkg.lock
COPY Gopkg.toml Gopkg.toml

RUN dep ensure --vendor-only

COPY cmd/	cmd/

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o prom-rule-reloader github.com/pusher/prom-rule-reloader/cmd

FROM alpine:3.8
RUN apk --no-cache add ca-certificates
WORKDIR /bin
COPY --from=builder /go/src/github.com/pusher/prom-rule-reloader/prom-rule-reloader .

ENTRYPOINT ["/bin/prom-rule-reloader"]
