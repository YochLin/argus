#!/usr/bin/env bash
#
# Installs and registers a GitHub Actions self-hosted runner on a VPS set up
# by scripts/vps/setup-env.sh (whether or not the box itself came from
# scripts/vps/create-vultr-instance.sh), and installs deploy/argus.service
# as a systemd --user unit. .github/workflows/deploy.yml runs on
# `runs-on: self-hosted` and does `systemctl --user restart argus`, so both
# of these need to exist on the box before the first deploy can succeed.
#
# Also installs deploy/shioaji.service (pipx install shioaji + enable --now)
# when ~/apps/argus/.env already has a real SJ_API_KEY — unlike claude/agy,
# the shioaji daemon logs in on its own from SJ_API_KEY/SJ_SEC_KEY in .env,
# no interactive OAuth, so there's nothing here that has to stay manual (see
# the sinopac-shioaji-integration memory: the old "needs a human present for
# token_pool decryption" assumption was disproven — it stays logged in
# unattended). Skipped silently when .env isn't there yet or SJ_API_KEY is
# still the .env.example placeholder, since Shioaji is optional.
#
# Run this from your local machine (needs `gh`, logged in with a token that
# has repo admin rights, to mint a short-lived runner registration token).
#
# Usage: scripts/vps/setup-runner.sh <ip> [user]
set -euo pipefail

IP="${1:?usage: setup-runner.sh <ip> [user]}"
APP_USER="${2:-argus}"
REPO="${GH_REPO:-YochLin/argus}"

command -v gh >/dev/null || { echo "gh CLI required (and logged in) to mint a runner registration token" >&2; exit 1; }

echo "==> minting a runner registration token for $REPO"
REG_TOKEN="$(gh api -X POST "repos/${REPO}/actions/runners/registration-token" --jq .token)"

RUNNER_VERSION="$(gh api repos/actions/runner/releases/latest --jq .tag_name | sed 's/^v//')"
echo "==> installing runner v${RUNNER_VERSION} on ${APP_USER}@${IP}"

ssh "${APP_USER}@${IP}" bash -s -- "$REPO" "$REG_TOKEN" "$RUNNER_VERSION" <<'REMOTE'
set -euo pipefail
REPO="$1"; TOKEN="$2"; VER="$3"
mkdir -p ~/actions-runner && cd ~/actions-runner
curl -fsSL -o runner.tar.gz "https://github.com/actions/runner/releases/download/v${VER}/actions-runner-linux-x64-${VER}.tar.gz"
tar xzf runner.tar.gz && rm runner.tar.gz
./config.sh --url "https://github.com/${REPO}" --token "$TOKEN" --unattended --replace --name "$(hostname)-argus"
sudo ./svc.sh install
sudo ./svc.sh start
REMOTE

echo "==> installing argus.service (systemd --user unit)"
scp "$(cd "$(dirname "$0")/../.." && pwd)/deploy/argus.service" "${APP_USER}@${IP}:/tmp/argus.service"
ssh "${APP_USER}@${IP}" bash -s <<'REMOTE'
set -euo pipefail
mkdir -p ~/.config/systemd/user
mv /tmp/argus.service ~/.config/systemd/user/argus.service
export XDG_RUNTIME_DIR="/run/user/$(id -u)"
systemctl --user daemon-reload
systemctl --user enable argus
REMOTE

echo "==> checking for a real SJ_API_KEY in .env (Shioaji setup, optional)"
scp "$(cd "$(dirname "$0")/../.." && pwd)/deploy/shioaji.service" "${APP_USER}@${IP}:/tmp/shioaji.service"
ssh "${APP_USER}@${IP}" bash -s <<'REMOTE'
set -euo pipefail
ENV_FILE=~/apps/argus/.env
SJ_KEY="$(grep '^SJ_API_KEY=' "$ENV_FILE" 2>/dev/null | cut -d= -f2- || true)"

if [ -z "$SJ_KEY" ] || [ "$SJ_KEY" = "your_sinopac_api_key_here" ]; then
  echo "no real SJ_API_KEY in .env yet -- skipping Shioaji (write .env, then re-run this script to pick it up)"
  rm -f /tmp/shioaji.service
  exit 0
fi

command -v pipx >/dev/null || sudo apt-get install -y pipx
pipx install --force shioaji
pipx ensurepath

mkdir -p ~/.config/systemd/user
mv /tmp/shioaji.service ~/.config/systemd/user/shioaji.service
export XDG_RUNTIME_DIR="/run/user/$(id -u)"
systemctl --user daemon-reload
# argus.service waits for the first CI deploy to have a binary (enable-only,
# above); shioaji's binary is already on disk from pipx and logs itself in,
# so it's safe to start immediately.
systemctl --user enable --now shioaji
echo "shioaji: pipx-installed and started"
REMOTE

cat <<EOF

==> done. Remaining manual steps on ${IP} (can't be scripted):
  1. mkdir -p ~/apps/argus && cd ~/apps/argus
     write .env there (see .env.example in the repo for the full list —
     TELEGRAM_BOT_TOKEN/TELEGRAM_CHAT_ID are required). Re-run this script
     afterward if you want the Shioaji step above to pick up SJ_API_KEY.
  2. claude   # log in once with your Pro/Max account (interactive OAuth)
  3. (optional) npm install -g @agentclientprotocol/claude-agent-acp
     then set CLAUDE_ACP_COMMAND=claude-agent-acp in .env to skip npx resolve overhead
  4. (optional, only if you set ANTIGRAVITY_ENABLED=true in .env) agy
     # log in once with your Google Antigravity account (interactive OAuth) —
     # the --sandbox runtime requirement noted in internal/llm/antigravity_provider.go
     # is still unverified against a real install, watch the first fallback call closely
  5. trigger .github/workflows/deploy.yml (workflow_dispatch) once to build
     the binary and start the bot for the first time
EOF
