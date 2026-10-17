#!/bin/bash
# Makes Docker usable in Claude Code cloud sessions, so ./verify.sh and
# ./preview.sh work there. Does nothing anywhere else.
#
# Cloud sessions send HTTPS through a proxy that re-signs TLS with its own CA,
# which the stock base images don't trust, so `go mod download` and `npm ci`
# fail inside containers. This:
#   1. starts dockerd if it isn't running, pulling through a registry mirror;
#   2. rebuilds the base images that download things (Go, Node, Playwright)
#      locally under their own tags, with that CA trusted;
#   3. makes Compose use the classic builder, which uses those local images
#      (BuildKit always re-resolves base images from the registry).
# Nothing here changes how the app is built anywhere else.
set -euo pipefail

if [[ "${CLAUDE_CODE_REMOTE:-}" != "true" ]]; then
  exit 0
fi

ca=/root/.ccr/ca-bundle.crt
if [[ ! -f $ca ]] || ! command -v dockerd >/dev/null; then
  echo "session-start: no proxy CA or no dockerd; skipping Docker setup" >&2
  exit 0
fi

cd "$CLAUDE_PROJECT_DIR"

# 1. Start the daemon. Docker Hub rate-limits the sandbox's shared IP (429),
# so pull Hub images through Google's public mirror; Docker falls back to the
# Hub for anything the mirror lacks.
if ! docker info >/dev/null 2>&1; then
  nohup dockerd --registry-mirror=https://mirror.gcr.io >/tmp/dockerd.log 2>&1 &
  for _ in $(seq 1 30); do
    docker info >/dev/null 2>&1 && break
    sleep 1
  done
  docker info >/dev/null 2>&1 || { echo "session-start: dockerd did not start, see /tmp/dockerd.log" >&2; exit 1; }
fi

# 2. Base images with the CA trusted. The label records which CA an image
# holds, so a rerun skips images that are already done and redoes them if the
# CA changes.
ca_sum=$(sha256sum "$ca" | cut -c1-16)
# The tags come from the files that pin them, so version bumps (Dependabot's
# included) need no change here: the Dockerfile's Go and Node stages (the Go
# tests use the same Go image) and the Playwright version the tests pin.
pw_version=$(sed -n 's/.*"@playwright\/test": *"\([0-9.]*\)".*/\1/p' e2e/package.json)
mapfile -t images < <(sed -n 's/^FROM \(\(golang\|node\):[^ ]*\).*/\1/p' Dockerfile)
images+=("mcr.microsoft.com/playwright:v${pw_version}-noble")

ctx=$(mktemp -d)
trap 'rm -rf "$ctx"' EXIT
cp "$ca" "$ctx/proxy-ca.crt"

for img in "${images[@]}"; do
  have=$(docker image inspect -f '{{index .Config.Labels "ccr.proxy-ca"}}' "$img" 2>/dev/null || true)
  [[ $have == "$ca_sum" ]] && continue
  docker pull -q "$img" >/dev/null
  cat >"$ctx/Dockerfile" <<EOF
FROM $img
USER root
COPY proxy-ca.crt /usr/local/share/ca-certificates/proxy-ca.crt
RUN cat /usr/local/share/ca-certificates/proxy-ca.crt >> /etc/ssl/certs/ca-certificates.crt
ENV NODE_EXTRA_CA_CERTS=/usr/local/share/ca-certificates/proxy-ca.crt SSL_CERT_FILE=/etc/ssl/certs/ca-certificates.crt
LABEL ccr.proxy-ca=$ca_sum
EOF
  # Classic builder: BuildKit would fetch $img from the registry again. Its
  # deprecation notice is noise, so show output only on failure.
  if ! out=$(DOCKER_BUILDKIT=0 docker build -q -t "$img" "$ctx" 2>&1); then
    echo "$out" >&2
    exit 1
  fi
  echo "session-start: $img now trusts the proxy CA"
done

# 3. Classic builder for every command in the session.
if [[ -n "${CLAUDE_ENV_FILE:-}" ]]; then
  echo 'export DOCKER_BUILDKIT=0 COMPOSE_BAKE=false' >>"$CLAUDE_ENV_FILE"
fi
