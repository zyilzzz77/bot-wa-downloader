FROM golang:alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY *.go ./
COPY scripts ./scripts
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o botgodownloader .

FROM alpine:3.22

# ffmpeg -> fitur brat/bratvid
# bash + wget -> fitur /bench (script bash + unduh binary speedtest)
# gcompat + libstdc++ + libgcc -> agar binary Ookla speedtest (glibc) jalan di musl
RUN apk add --no-cache ca-certificates tzdata ffmpeg bash wget curl gcompat libstdc++ libgcc

# Ookla Speedtest CLI (dipakai fitur /bench)
RUN set -eux; \
    case "$(uname -m)" in \
        x86_64|amd64) st=x86_64 ;; \
        aarch64|arm64) st=aarch64 ;; \
        armv7l) st=armhf ;; \
        *) st=x86_64 ;; \
    esac; \
    wget -qO /tmp/speedtest.tgz "https://install.speedtest.net/app/cli/ookla-speedtest-1.2.0-linux-${st}.tgz"; \
    mkdir -p /app/speedtest-cli; \
    tar -xzf /tmp/speedtest.tgz -C /app/speedtest-cli; \
    chmod +x /app/speedtest-cli/speedtest; \
    rm -f /tmp/speedtest.tgz

WORKDIR /app
COPY --from=builder /app/botgodownloader .

RUN mkdir -p /app/data

ENTRYPOINT ["./botgodownloader"]
