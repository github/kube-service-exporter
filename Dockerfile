FROM golang:1.10-stretch as builder

ARG CONSUL_URL="https://releases.hashicorp.com/consul/1.0.6/consul_1.0.6_linux_amd64.zip"
ARG CONSUL_SHA="bcc504f658cef2944d1cd703eda90045e084a15752d23c038400cf98c716ea01"
RUN apt-get update && \
    apt-get install -y \
      git \
      unzip
RUN curl -s "$CONSUL_URL" -o /tmp/consul.zip && \
      echo "$CONSUL_SHA /tmp/consul.zip" | sha256sum -c && \
      unzip /tmp/consul.zip -d /usr/local/bin

WORKDIR /go/src/github.com/github/kube-service-exporter
COPY . .
COPY .git .
RUN make

FROM debian:stretch-slim 
COPY --from=builder /go/src/github.com/github/kube-service-exporter/bin/kube-service-exporter /usr/local/bin
ENTRYPOINT ["/usr/local/bin/kube-service-exporter"]
