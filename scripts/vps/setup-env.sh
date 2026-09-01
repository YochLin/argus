#!/usr/bin/env bash
#
# Sets up a fresh Ubuntu box with what argus needs to build & run: Go,
# Node.js (npx — internal/llm shells out to claude-agent-acp via it), the
# `claude` CLI itself (its login writes the credentials claude-agent-acp
# reads — the Agent SDK it depends on has no bundled auth of its own), the
# `agy` CLI (internal/llm's optional ANTIGRAVITY_ENABLED fallback — see
# internal/llm/antigravity_provider.go; installed either way since it's
# harmless at rest, but stays inert unless that env var is set and it's
# separately logged in), git, a non-root APP_USER with `sudo` + systemd
# linger enabled (linger is what lets a `systemctl --user` service, see
# deploy/argus.service, keep running after the SSH session that started it
# ends), a 5GB swapfile (every box this project has run on has needed one),
# and the `openvpn` package (every box connects out through the home-router
# full tunnel — see docs/vps-migration-hostinger.md in argus-storage; this
# script only installs the package, copying over the actual client.conf and
# enabling the service is still a manual step since the cert lives on
# whichever box is already connected).
#
# Works against ANY box you can already SSH into as a user with sudo (or
# root) — one created by scripts/vps/create-vultr-instance.sh, one clicked
# together in Vultr's web console, or a VPS from a different provider
# entirely. That's the point of the split from the old provision.sh: the
# Vultr-API part and the inside-the-box part are independent now, so a
# manually-created VPS can skip straight to this script.
#
# This script only sets up the box. It does NOT install/register the GitHub
# Actions self-hosted runner or start the bot — run scripts/vps/setup-runner.sh
# <ip> next, then follow README's "Getting started" for .env + the one-time
# `claude` CLI login (that part can't be scripted, it's an interactive OAuth
# flow).
#
# Usage: scripts/vps/setup-env.sh <ip> [remote_user] [app_user]
#   remote_user - who you SSH in as to run this setup (default: root — what
#                 create-vultr-instance.sh's box boots with; use whatever
#                 sudo-capable user your manually-created box already has)
#   app_user    - the non-root user created/configured to actually run the
#                 bot (default: argus, or $VPS_USER)
set -euo pipefail

IP="${1:?usage: setup-env.sh <ip> [remote_user] [app_user]}"
REMOTE_USER="${2:-root}"
APP_USER="${3:-${VPS_USER:-argus}}"
SSH_PUBKEY_PATH="${SSH_PUBKEY_PATH:-$HOME/.ssh/id_rsa.pub}"

[ -f "$SSH_PUBKEY_PATH" ] || { echo "no pubkey at $SSH_PUBKEY_PATH (set SSH_PUBKEY_PATH)" >&2; exit 1; }
PUBKEY="$(cat "$SSH_PUBKEY_PATH")"

echo "==> setting up ${APP_USER}@${IP} (connecting as ${REMOTE_USER})"
# ssh doesn't preserve local argv quoting for a remote command — it just
# joins "bash -s -- <arg1> <arg2>" with spaces and hands that to the
# remote shell to reparse. Passing APP_USER/PUBKEY as-is would break the
# moment either contains a shell metacharacter (an apostrophe in the local
# key's comment, e.g. "user@Jane's-MacBook", is enough) — base64 has no such
# characters, so it survives the round trip regardless of what's inside.
APP_USER_B64="$(printf '%s' "$APP_USER" | base64 | tr -d '\n')"
PUBKEY_B64="$(printf '%s' "$PUBKEY" | base64 | tr -d '\n')"
ssh "${REMOTE_USER}@${IP}" bash -s -- "$APP_USER_B64" "$PUBKEY_B64" <<'REMOTE'
set -euo pipefail
APP_USER="$(printf '%s' "$1" | base64 -d)"; PUBKEY="$(printf '%s' "$2" | base64 -d)"

export DEBIAN_FRONTEND=noninteractive
apt-get update -y
apt-get upgrade -y
apt-get install -y git curl build-essential unzip jq openvpn

# Swap — every box this project has run on needed it (npx + the claude CLI
# are memory-hungry relative to a 1-2GB VPS); apt/cloud-init doesn't set one
# up on its own. Skip if a swapfile is already configured (re-running this
# script on a box that has one shouldn't fallocate a second).
if ! swapon --show=NAME --noheadings | grep -q .; then
  fallocate -l 5G /swapfile
  chmod 600 /swapfile
  mkswap /swapfile
  swapon /swapfile
  grep -qxF '/swapfile swap swap defaults 0 0' /etc/fstab || \
    echo '/swapfile swap swap defaults 0 0' >> /etc/fstab
fi

if ! id -u "$APP_USER" >/dev/null 2>&1; then
  useradd -m -s /bin/bash -G sudo "$APP_USER"
fi
echo "${APP_USER} ALL=(ALL) NOPASSWD:ALL" > "/etc/sudoers.d/90-${APP_USER}"
chmod 440 "/etc/sudoers.d/90-${APP_USER}"

install -d -m 700 -o "$APP_USER" -g "$APP_USER" "/home/${APP_USER}/.ssh"
touch "/home/${APP_USER}/.ssh/authorized_keys"
grep -qxF "$PUBKEY" "/home/${APP_USER}/.ssh/authorized_keys" || echo "$PUBKEY" >> "/home/${APP_USER}/.ssh/authorized_keys"
chmod 600 "/home/${APP_USER}/.ssh/authorized_keys"
chown "${APP_USER}:${APP_USER}" "/home/${APP_USER}/.ssh/authorized_keys"

loginctl enable-linger "$APP_USER"
mkdir -p "/home/${APP_USER}/apps/argus/data"
chown -R "${APP_USER}:${APP_USER}" "/home/${APP_USER}/apps"

# Go: fetch whatever's currently stable rather than pinning a version here,
# since apt's go package lags upstream by months.
GOVER=$(curl -fsSL https://go.dev/VERSION?m=text | head -1)
curl -fsSL "https://go.dev/dl/${GOVER}.linux-amd64.tar.gz" -o /tmp/go.tar.gz
rm -rf /usr/local/go
tar -C /usr/local -xzf /tmp/go.tar.gz
rm /tmp/go.tar.gz
grep -qxF 'export PATH=$PATH:/usr/local/go/bin' "/home/${APP_USER}/.bashrc" 2>/dev/null || \
  echo 'export PATH=$PATH:/usr/local/go/bin' >> "/home/${APP_USER}/.bashrc"
chown "${APP_USER}:${APP_USER}" "/home/${APP_USER}/.bashrc"

# Node.js LTS — npx is what internal/llm shells out to for claude-agent-acp
curl -fsSL https://deb.nodesource.com/setup_lts.x | bash -
apt-get install -y nodejs

# The `claude` CLI itself — installing it is scriptable, logging in isn't
# (interactive OAuth, see this script's header comment and the printed
# next-steps in setup-runner.sh).
npm install -g @anthropic-ai/claude-code

# The `agy` CLI (Antigravity, internal/llm's optional LLM fallback). The
# official installer drops the binary in the invoking user's ~/.local/bin,
# so it's run as APP_USER (not root) or it'd land in /root and be invisible
# to both APP_USER's shell and the systemd --user service that actually
# needs it. That service's default PATH doesn't include ~/.local/bin
# either, hence the /usr/local/bin symlink — see deploy/argus.service.
sudo -u "$APP_USER" -H bash -c 'curl -fsSL https://antigravity.google/cli/install.sh | bash'
ln -sf "/home/${APP_USER}/.local/bin/agy" /usr/local/bin/agy
REMOTE

echo
echo "==> ${APP_USER}@${IP} is ready."
echo "Next: scripts/vps/setup-runner.sh $IP $APP_USER"
