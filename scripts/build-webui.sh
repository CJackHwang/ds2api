#!/bin/bash
# WebUI 构建脚本
# 用法: ./scripts/build-webui.sh

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
WEBUI_DIR="$ROOT_DIR/webui"
OUT_DIR="${DS2API_STATIC_ADMIN_DIR:-$ROOT_DIR/static/admin}"
NPM_REGISTRY="${NPM_REGISTRY:-https://registry.npmmirror.com}"

echo "🔨 Building WebUI..."
echo "📁 WebUI:   $WEBUI_DIR"
echo "📁 Output:  $OUT_DIR"

cd "$WEBUI_DIR"

if [ ! -d "node_modules" ]; then
    echo "📦 Installing dependencies with npm ci..."
    npm ci --include=optional --registry "$NPM_REGISTRY"
fi

echo "🏗️  Running build..."
npm run build -- --outDir "$OUT_DIR" --emptyOutDir

if [ ! -f "$OUT_DIR/index.html" ]; then
    echo "❌ WebUI build failed: $OUT_DIR/index.html not found"
    exit 1
fi

echo "✅ WebUI built successfully!"
echo "📁 Output: $OUT_DIR"
