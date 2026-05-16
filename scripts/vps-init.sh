#!/usr/bin/env bash
set -euo pipefail

red='\033[0;31m' green='\033[0;32m' nc='\033[0m'
step() { echo -e "${green}→${nc} $*"; }
skip() { echo -e "  ${green}✓${nc} $*"; }

# ── 1. Install Docker ─────────────────────────────────
if command -v docker &>/dev/null; then
  skip "Docker $(docker --version | awk '{print $3}' | tr -d ',')"
else
  step "Installing Docker..."
  curl -fsSL https://get.docker.com | sh
  systemctl enable docker --now
fi

# ── 2. GHCR authentication ────────────────────────────
if grep -q "ghcr.io" ~/.docker/config.json 2>/dev/null; then
  skip "GHCR credentials found"
else
  step "Authenticating with GHCR"
  echo "  Enter a GitHub PAT with read:packages scope:"
  docker login ghcr.io -u Sign0ret
fi

# ── 3. Directory layout ───────────────────────────────
mkdir -p /opt/camaron /data/camaron /tmp/chunks

# ── 4. Compose file (first-run only) ──────────────────
COMPOSE_FILE=/opt/camaron/docker-compose.yml
if [ -f "$COMPOSE_FILE" ]; then
  skip "$COMPOSE_FILE exists (skipping overwrite)"
else
  step "Writing $COMPOSE_FILE"
  cat > "$COMPOSE_FILE" <<'COMPOSE'
services:
  orchestrator:
    image: ghcr.io/Sign0ret/camaron-orchestrator:latest
    ports:
      - "8080:8080"
    volumes:
      - /data/camaron:/data/camaron
      - /tmp/chunks:/tmp/chunks
    restart: always

  watchtower:
    image: containrrr/watchtower:latest
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
    environment:
      WATCHTOWER_POLL_INTERVAL: 300
      WATCHTOWER_CLEANUP: "true"
      WATCHTOWER_INCLUDE_SELF: "true"
    restart: always
COMPOSE
fi

# ── 5. Start services ─────────────────────────────────
step "Starting services..."
cd /opt/camaron && docker compose up -d

# ── 6. Firewall ────────────────────────────────────────
if command -v ufw &>/dev/null; then
  ufw allow 8080/tcp comment 'camaron-orchestrator' 2>/dev/null || true
  ufw status | grep -q "Status: active" || { step "Enabling firewall..."; ufw --force enable; }
fi

# ── 7. Report ──────────────────────────────────────────
echo ""
echo -e "${green}✓ VPS ready${nc}"
echo "  Health:  http://$(hostname -I | awk '{print $1}'):8080/health"
echo "  Compose: $COMPOSE_FILE"
