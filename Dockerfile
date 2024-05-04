FROM golang:1.22-bookworm AS builder

WORKDIR /app
COPY go.mod go.sum main.go ./
RUN go mod download
RUN GOOS=linux go build -o speedtester .

FROM debian:bookworm
RUN apt update && \
  apt install wget -y && \
  wget -q -O /tmp/setup.sh https://packagecloud.io/install/repositories/ookla/speedtest-cli/script.deb.sh && \
  chmod +x /tmp/setup.sh && \
  /tmp/setup.sh && \
  rm /tmp/setup.sh && \
  apt install speedtest -y && \
  useradd -m speedtester
COPY --from=builder /app/speedtester /usr/local/bin/speedtester
USER speedtester
ENTRYPOINT [ "/usr/local/bin/speedtester" ]
