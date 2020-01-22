FROM golang:1.13-buster as builder

ARG CONSUL_URL="https://releases.hashicorp.com/consul/1.6.1/consul_1.6.1_linux_amd64.zip"
ARG CONSUL_SHA="a8568ca7b6797030b2c32615b4786d4cc75ce7aee2ed9025996fe92b07b31f7e"
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

FROM debian:buster-slim
COPY --from=builder /go/src/github.com/github/kube-service-exporter/bin/kube-service-exporter /usr/local/bin
ENTRYPOINT ["/usr/local/bin/kube-service-exporter"]
