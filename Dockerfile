# syntax=docker/dockerfile:1

# ---- build: a static Go binary, cross-compiled for the target platform ----
FROM --platform=$BUILDPLATFORM golang:1.27.4-alpine AS build
WORKDIR /src

# Download modules first, so code changes don't invalidate this layer.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .
ARG TARGETOS TARGETARCH
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -buildvcs=false -pgo=auto -ldflags="-s -w" -o /out/ytgrab ./cmd/ytgrab

# ---- runtime: yt-dlp (Python) + ffmpeg + the binary ----
FROM python:3.13-alpine

# Pin yt-dlp for reproducible builds. Bump it when YouTube changes break it:
#   docker build --build-arg YTDLP_VERSION=<new version> .
ARG YTDLP_VERSION=2026.11.23
RUN apk add --no-cache ffmpeg tini \
 && pip install --no-cache-dir "yt-dlp==${YTDLP_VERSION}" \
 && adduser -D -u 10001 ytgrab \
 && mkdir -p /data \
 && chown ytgrab:ytgrab /data

COPY --from=build /out/ytgrab /usr/local/bin/ytgrab
COPY deploy/ytgrab.yml /etc/ytgrab/ytgrab.yml

USER ytgrab
WORKDIR /data
VOLUME /data

# tini is PID 1: it forwards SIGTERM/SIGINT to ytgrab (clean stop on
# `docker stop` or an ECS task stop) and reaps orphaned ffmpeg processes.
ENTRYPOINT ["/sbin/tini", "--", "ytgrab", "-c", "/etc/ytgrab/ytgrab.yml"]
# Default input: /data/urls.txt. Override with any file under /data, or "-" for stdin.
CMD ["/data/urls.txt"]
