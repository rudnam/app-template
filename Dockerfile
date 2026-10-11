# syntax=docker/dockerfile:1

# ---- Web: type-check and bundle the React app into web/dist ----
FROM node:24-alpine AS web
WORKDIR /web
# Install dependencies first so this layer stays cached until the lockfile changes.
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

# ---- Build: compile a static binary with the web app embedded ----
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /web/dist ./web/dist
# Shown by /healthz and in the startup log, so you can tell which build runs.
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/server ./cmd/server

# ---- Runtime: no shell, no package manager, non-root ----
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/server /server
USER nonroot:nonroot
EXPOSE 8080
ENV ADDR=:8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 CMD ["/server", "healthcheck"]
ENTRYPOINT ["/server"]
