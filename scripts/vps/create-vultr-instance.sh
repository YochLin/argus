#!/usr/bin/env bash
#
# Creates a fresh Vultr VPS via the Vultr API — nothing more. It does NOT
# install anything inside the box (no Go/Node/git, no app user); that's
# scripts/vps/setup-env.sh's job, which works the same whether the VPS came
# from this script or you built one by hand (Vultr's own web console, or any
# other provider).
#
# The instance boots with only the OS's default user (root, for Ubuntu) and
# your SSH public key installed via Vultr's sshkey_id — no cloud-init.
#
# Requires: VULTR_API_KEY (https://my.vultr.com/settings/#settingsapi), curl, jq.
#
# Usage: VULTR_API_KEY=... scripts/vps/create-vultr-instance.sh
set -euo pipefail

: "${VULTR_API_KEY:?Set VULTR_API_KEY (create one at https://my.vultr.com/settings/#settingsapi)}"

REGION="${VULTR_REGION:-nrt}"        # Tokyo — lowest-latency Vultr region to Taiwan
PLAN="${VULTR_PLAN:-vc2-1c-2gb}"     # 1 vCPU / 2GB: headroom for `go build` + npx + sqlite, still cheap
LABEL="${VULTR_LABEL:-argus-vps}"
SSH_PUBKEY_PATH="${SSH_PUBKEY_PATH:-$HOME/.ssh/id_rsa.pub}"
OS_NAME="${VULTR_OS_NAME:-Ubuntu 24.04 LTS x64}"

API="https://api.vultr.com/v2"
auth=(-H "Authorization: Bearer ${VULTR_API_KEY}")
json=(-H "Content-Type: application/json")

command -v jq >/dev/null || { echo "jq required (brew install jq)" >&2; exit 1; }
[ -f "$SSH_PUBKEY_PATH" ] || { echo "no pubkey at $SSH_PUBKEY_PATH (set SSH_PUBKEY_PATH)" >&2; exit 1; }
PUBKEY="$(cat "$SSH_PUBKEY_PATH")"

echo "==> looking up os_id for '${OS_NAME}'"
OS_ID="$(curl -fsSL "${auth[@]}" "$API/os" | jq -r --arg n "$OS_NAME" '.os[] | select(.name==$n) | .id' | head -1)"
[ -n "$OS_ID" ] || { echo "couldn't find os '$OS_NAME' — check $API/os" >&2; exit 1; }

echo "==> registering ssh key with Vultr (reusing it if already present)"
SSHKEY_ID="$(curl -fsSL "${auth[@]}" "$API/ssh-keys" | jq -r --arg k "$PUBKEY" '.ssh_keys[] | select(.ssh_key==$k) | .id' | head -1)"
if [ -z "$SSHKEY_ID" ]; then
  SSHKEY_ID="$(curl -fsSL -X POST "${auth[@]}" "${json[@]}" "$API/ssh-keys" \
    -d "$(jq -n --arg name "$LABEL" --arg key "$PUBKEY" '{name:$name, ssh_key:$key}')" \
    | jq -r '.ssh_key.id')"
fi

echo "==> creating instance ($PLAN in $REGION)"
INSTANCE_JSON="$(curl -fsSL -X POST "${auth[@]}" "${json[@]}" "$API/instances" -d "$(jq -n \
  --arg region "$REGION" --arg plan "$PLAN" --argjson os_id "$OS_ID" --arg lbl "$LABEL" \
  --arg sshkey_id "$SSHKEY_ID" \
  '{region:$region, plan:$plan, os_id:$os_id, label:$lbl, sshkey_id:[$sshkey_id], backups:"disabled"}')")"

INSTANCE_ID="$(echo "$INSTANCE_JSON" | jq -r '.instance.id')"
[ -n "$INSTANCE_ID" ] && [ "$INSTANCE_ID" != "null" ] || { echo "create failed: $INSTANCE_JSON" >&2; exit 1; }
echo "==> instance $INSTANCE_ID created, waiting for it to get an IP..."

IP="0.0.0.0"
for _ in $(seq 1 60); do
  INFO="$(curl -fsSL "${auth[@]}" "$API/instances/$INSTANCE_ID")"
  IP="$(echo "$INFO" | jq -r '.instance.main_ip')"
  STATUS="$(echo "$INFO" | jq -r '.instance.server_status')"
  [ "$IP" != "0.0.0.0" ] && [ "$STATUS" = "ok" ] && break
  sleep 5
done
[ "$IP" != "0.0.0.0" ] || { echo "timed out waiting for instance to boot — check the Vultr dashboard" >&2; exit 1; }

echo
echo "==> instance ready: $IP (root, key-based ssh)"
echo "the box may still be finishing boot in the background — give it a minute if ssh isn't answering yet."
echo
echo "Next: scripts/vps/setup-env.sh $IP"
