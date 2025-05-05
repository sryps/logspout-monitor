FROM golang:1.23-bookworm AS build-env

WORKDIR /root

COPY . .
RUN go mod download
RUN go install .

FROM debian:bookworm-slim

# Install reporting requirements
RUN apt-get update -y && apt install jq ca-certificates -y && rm -rf /var/lib/apt/lists/*

RUN useradd -m monitor -s /bin/bash
WORKDIR /home/monitor
USER monitor:monitor

COPY --chown=0:0 --from=build-env /go/bin/logspout-monitor /usr/bin/monitor


ENTRYPOINT ["/usr/bin/monitor"]

CMD ["start"]
