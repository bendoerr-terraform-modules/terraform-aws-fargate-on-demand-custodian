FROM golang:1.24 AS builder

WORKDIR /src

COPY ./watcher-tcp.go .

RUN CGO_ENABLED=0 GOOS=linux go build watcher-tcp.go

FROM bash:5.2-alpine3.21
LABEL org.opencontainers.image.source="https://github.com/bendoerr-terraform-modules/terraform-aws-fargate-on-demand-custodian"
LABEL org.opencontainers.image.description="Ben's Terraform AWS Fargate on Demand Module Custodian Sidecar"
LABEL org.opencontainers.image.authors="https://github.com/bendoerr"
LABEL org.opencontainers.image.licenses=MIT

WORKDIR /

RUN apk update && \
    apk add --no-cache \
      jq=~1.7 \
      curl=~8.12 \
      cmd:script=~2.40 \
      cmd:ss=~6.11 \
      aws-cli=~2.22

COPY ./custodian \
     ./dns-updater \
     ./event-emitter \
     ./task-reaper \
     ./

COPY --from=builder /src/watcher-tcp ./watcher-tcp

ENTRYPOINT ["./custodian"]
