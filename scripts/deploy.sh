#!/usr/bin/env bash
set -euo pipefail

IMAGE="ghcr.io/sign0ret/camaron-orchestrator:latest"
STAGING_PORT=8081
HEALTH_PORT=8080
MAX_RETRIES=10
RETRY_INTERVAL=2

echo "→ Pulling $IMAGE..."
docker pull "$IMAGE"

echo "→ Starting staging container..."
docker rm -f orchestrator-staging 2>/dev/null || true
docker run -d --rm --name orchestrator-staging \
  -p "$STAGING_PORT":"$HEALTH_PORT" \
  --env-file /opt/camaron/.env \
  "$IMAGE"

echo "→ Health-checking staging..."
for i in $(seq 1 $MAX_RETRIES); do
  if curl -sf "http://localhost:$STAGING_PORT/health" > /dev/null 2>&1; then
    echo "  ✓ Staging healthy"
    break
  fi
  if [ "$i" -eq "$MAX_RETRIES" ]; then
    echo "  ✗ Staging failed health check"
    docker rm -f orchestrator-staging 2>/dev/null || true
    exit 1
  fi
  sleep "$RETRY_INTERVAL"
done

echo "→ Pulling latest stack images..."
cd /opt/camaron && docker compose -f docker-compose.prod.yml pull

echo "→ Swapping orchestrator..."
cd /opt/camaron && docker compose -f docker-compose.prod.yml up -d --force-recreate orchestrator

echo "→ Updating remaining services..."
cd /opt/camaron && docker compose -f docker-compose.prod.yml up -d

echo "→ Cleaning up staging..."
docker rm -f orchestrator-staging 2>/dev/null || true

echo "✓ Deploy complete"
