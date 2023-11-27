FROM golang:1.21 AS builder

WORKDIR /src
COPY ./watcher-tcp.go .
RUN CGO_ENABLED=0 GOOS=linux go build watcher-tcp.go

FROM amazon/aws-cli
LABEL org.opencontainers.image.source = "https://github.com/bendoerr-terraform-modules/terraform-aws-fargate-on-demand-custodian"

RUN yum install -y \
    jq \
    net-tools \
    iproute \
    python3 \
    && \
    yum clean all

COPY ./custodian \
     ./dns-updater \
     ./event-emitter \
     ./task-reaper \
     ./

COPY --from=builder /src/watcher-tcp ./watcher-tcp

ENTRYPOINT ["./custodian"]