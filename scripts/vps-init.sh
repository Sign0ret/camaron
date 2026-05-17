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
  docker login ghcr.io -u sign0ret
fi

# ── 3. Directory layout ───────────────────────────────
mkdir -p /opt/camaron /data/camaron /tmp/chunks

# ── 4. Deployment script ────────────────────────────────
DEPLOY_SCRIPT=/opt/camaron/deploy.sh
if [ -f /root/deploy.sh ]; then
  cp /root/deploy.sh "$DEPLOY_SCRIPT" && step "deploy.sh copied to $DEPLOY_SCRIPT"
elif [ -f "$DEPLOY_SCRIPT" ]; then
  skip "$DEPLOY_SCRIPT already exists"
else
  echo -e "  ${red}⚠ /root/deploy.sh not found — deploy script missing${nc}"
fi

# ── 5. Environment file ────────────────────────────────
ENV_FILE=/opt/camaron/.env
if [ -f /root/.env ]; then
  cp /root/.env "$ENV_FILE" && step ".env copied to $ENV_FILE"
elif [ -f "$ENV_FILE" ]; then
  skip "$ENV_FILE already exists"
else
  step "Creating empty $ENV_FILE"
  echo "# camaron environment variables" > "$ENV_FILE"
fi

# ── 6. Compose file (first-run only) ──────────────────
COMPOSE_FILE=/opt/camaron/docker-compose.yml
if [ -f "$COMPOSE_FILE" ]; then
  skip "$COMPOSE_FILE exists (skipping overwrite)"
else
  step "Writing $COMPOSE_FILE"
  cat > "$COMPOSE_FILE" <<'COMPOSE'
services:
  orchestrator:
    image: ghcr.io/sign0ret/camaron-orchestrator:latest
    ports:
      - "8080:8080"
    volumes:
      - /data/camaron:/data/camaron
      - /tmp/chunks:/tmp/chunks
    env_file: /opt/camaron/.env
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://localhost:8080/health"]
      interval: 10s
      timeout: 5s
      retries: 3
      start_period: 5s
    restart: always
COMPOSE
fi

# ── 7. Start services ─────────────────────────────────
step "Starting services..."
cd /opt/camaron && docker compose up -d

# ── 8. Firewall ────────────────────────────────────────
if command -v ufw &>/dev/null; then
  ufw allow 22/tcp comment 'ssh' 2>/dev/null || true
  ufw allow 8080/tcp comment 'camaron-orchestrator' 2>/dev/null || true
  ufw status | grep -q "Status: active" || { step "Enabling firewall..."; ufw --force enable; }
fi

# ── 9. Report ──────────────────────────────────────────
echo ""
echo -e "${green}✓ VPS ready${nc}"
echo "  Health:  http://$(hostname -I | awk '{print $1}'):8080/health"
echo "  Compose: $COMPOSE_FILE"
