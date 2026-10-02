FROM docker.io/oven/bun:1.3.14 AS panel

RUN apt-get update && apt-get install -y --no-install-recommends git ca-certificates && rm -rf /var/lib/apt/lists/*

WORKDIR /panel
ARG MANAGEMENT_UI_COMMIT=a9eda98ad825b166f6516f7b04c7b4a2b4f890de
RUN git init && git remote add origin https://github.com/rexlManu/Cli-Proxy-API-Management-Center.git && git fetch --depth 1 origin "${MANAGEMENT_UI_COMMIT}" && git checkout --detach FETCH_HEAD
RUN bun install --frozen-lockfile
RUN VERSION=quota-ledger-${MANAGEMENT_UI_COMMIT} bun run build

FROM docker.io/library/golang:1.26-bookworm AS builder

RUN apt-get update && apt-get install -y --no-install-recommends build-essential git && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .

ARG VERSION=earliest-reset
ARG COMMIT=none
ARG BUILD_DATE=unknown
RUN CGO_ENABLED=1 GOOS=linux go build -buildvcs=false -ldflags="-s -w -X 'main.Version=${VERSION}' -X 'main.Commit=${COMMIT}' -X 'main.BuildDate=${BUILD_DATE}'" -o /CLIProxyAPI ./cmd/server/

FROM docker.io/library/debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends tzdata ca-certificates && rm -rf /var/lib/apt/lists/*
WORKDIR /CLIProxyAPI
COPY --from=builder /CLIProxyAPI ./CLIProxyAPI
COPY --from=panel /panel/dist/index.html /opt/cliproxy-panel/management.html

ENV TZ=Etc/UTC MANAGEMENT_STATIC_PATH=/opt/cliproxy-panel
EXPOSE 8317
CMD ["./CLIProxyAPI", "--config", "/config/config.yaml"]
