#!/usr/bin/env sh
set -eu

REPO_URL="${ATO_CFIP_REPO:-https://github.com/clover-eric/ato-cfip.git}"
INSTALL_DIR="${ATO_CFIP_DIR:-$HOME/ato-cfip}"
WEB_PORT="${CFST_WEB_PORT:-8080}"

need() {
  command -v "$1" >/dev/null 2>&1
}

echo "==> ATO-CFIP one-click installer"
echo "==> Install dir: $INSTALL_DIR"

if ! need git; then
  echo "git is required. Please install git first."
  exit 1
fi

if ! need docker; then
  echo "Docker is required. Please install Docker / Container Manager on your NAS first."
  exit 1
fi

if docker compose version >/dev/null 2>&1; then
  COMPOSE="docker compose"
elif need docker-compose; then
  COMPOSE="docker-compose"
else
  echo "Docker Compose is required."
  exit 1
fi

if [ -d "$INSTALL_DIR/.git" ]; then
  echo "==> Updating existing checkout"
  git -C "$INSTALL_DIR" pull --ff-only
else
  echo "==> Cloning repository"
  git clone "$REPO_URL" "$INSTALL_DIR"
fi

cd "$INSTALL_DIR"

if [ ! -f .env ]; then
  cp .env.example .env
  if [ "$WEB_PORT" != "8080" ]; then
    sed -i.bak "s/^CFST_WEB_PORT=.*/CFST_WEB_PORT=$WEB_PORT/" .env && rm -f .env.bak
  fi
fi

if [ ! -f config.yaml ]; then
  cp config.example.yaml config.yaml
  echo "==> Created config.yaml"
  echo "==> Edit config.yaml to set your domain and Cloudflare API token when ready."
fi

mkdir -p data

echo "==> Starting service"
$COMPOSE up -d --build

echo ""
echo "ATO-CFIP is running."
echo "Open: http://YOUR_NAS_IP:$WEB_PORT"
echo "Config: $INSTALL_DIR/config.yaml"

