# syntax=docker/dockerfile:1

# The runtime must share a glibc generation with the host that runs argus: the k3s
# manifest bind-mounts the host's whole home, and ~/.local/bin/agy (the Antigravity
# fallback) is a dynamically linked binary built for the host. An older glibc
# (debian-slim's 2.36) isn't guaranteed to run it. The Go binary is static, so it
# doesn't care.
ARG RUNTIME_BASE=ubuntu:26.04

# 1. Frontend. Vite's outDir is ../internal/web/dist (go:embed can't reach outside its
#    own package), so keep the same relative layout here: after the build,
#    /src/internal/web/dist is what the Go stage needs.
FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# 2. Go. The dist copy must come after `COPY . .`, otherwise the placeholder
#    index.html committed in the repo overwrites it, and the binary builds fine but
#    serves an empty dashboard with no error anywhere.
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/internal/web/dist ./internal/web/dist
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /argus ./cmd/server

# 3. Runtime. Node is needed because llm.acpProvider shells out to claude-agent-acp;
#    installing it globally up front avoids npx resolving it on every call (see
#    CLAUDE_ACP_COMMAND).
FROM ${RUNTIME_BASE}
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates nodejs npm \
 && npm install -g @agentclientprotocol/claude-agent-acp \
 && rm -rf /var/lib/apt/lists/* /root/.npm
# Not under /app: the k3s manifest mounts a host directory over the working
# directory, which would hide a binary placed there.
COPY --from=builder /argus /usr/local/bin/argus
ENV CLAUDE_ACP_COMMAND=claude-agent-acp
WORKDIR /app
RUN mkdir -p /app/data
ENTRYPOINT ["/usr/local/bin/argus"]
