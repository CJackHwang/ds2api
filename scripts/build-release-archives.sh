#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT_DIR"

source "${ROOT_DIR}/scripts/release-targets.sh"

show_targets() {
  echo "可用构建目标："
  echo ""
  local i=1
  for target in "${DS2API_RELEASE_TARGETS[@]}"; do
    read -r goos goarch goarm label <<< "$target"
    printf "  [%2d] %-12s (%s/%s%s)\n" "$i" "$label" "$goos" "$goarch" "${goarm:+/arm$goarm}"
    i=$((i+1))
  done
  echo ""
  echo "用法："
  echo "  ./scripts/build-release-archives.sh              # 交互式选择"
  echo "  ./scripts/build-release-archives.sh --all       # 构建所有目标"
  echo "  ./scripts/build-release-archives.sh 1 3 5       # 构建指定序号的目标"
  echo "  ./scripts/build-release-archives.sh darwin_arm64 # 构建指定标签的目标"
  echo ""
}

select_targets() {
  if [[ "${1:-}" == "--all" ]]; then
    printf '%s\n' "${DS2API_RELEASE_TARGETS[@]}"
    return
  fi

  local selected=()
  for arg in "$@"; do
    if [[ "$arg" =~ ^[0-9]+$ ]]; then
      local target="${DS2API_RELEASE_TARGETS[$((arg-1))]:-}"
      if [[ -n "$target" ]]; then
        selected+=("$target")
      fi
    else
      for target in "${DS2API_RELEASE_TARGETS[@]}"; do
        read -r _ _ _ label <<< "$target"
        if [[ "$label" == "$arg" ]]; then
          selected+=("$target")
          break
        fi
      done
    fi
  done

  if [[ ${#selected[@]} -eq 0 ]]; then
    echo "请选择构建目标：" >&2
    local i=1 choices=""
    for target in "${DS2API_RELEASE_TARGETS[@]}"; do
      read -r goos goarch goarm label <<< "$target"
      echo "  [$i] $label" >&2
      i=$((i+1))
    done
    echo "" >&2
    echo "输入序号（空格分隔，如 4 5）：" >&2
    read -r choices </dev/tty
    for num in $choices; do
      local target="${DS2API_RELEASE_TARGETS[$((num-1))]:-}"
      if [[ -n "$target" ]]; then
        selected+=("$target")
      fi
    done
  fi

  printf '%s\n' "${selected[@]}"
}

build_one() {
  local tag="$1" build_version="$2" goos="$3" goarch="$4" goarm="$5" label="$6"
  local pkg stage bin

  pkg="ds2api_${tag}_${label}"
  stage="dist/${pkg}"
  bin="ds2api"
  if [[ "$goos" == "windows" ]]; then
    bin="ds2api.exe"
  fi

  echo "[release-archives] building ${label}"
  rm -rf "$stage"
  mkdir -p "${stage}/static"

  if [[ "$goarm" == "-" ]]; then
    CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
      go build -buildvcs=false -trimpath -ldflags="-s -w -X ds2api/internal/version.BuildVersion=${build_version}" -o "${stage}/${bin}" ./cmd/ds2api
  else
    CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" GOARM="$goarm" \
      go build -buildvcs=false -trimpath -ldflags="-s -w -X ds2api/internal/version.BuildVersion=${build_version}" -o "${stage}/${bin}" ./cmd/ds2api
  fi

  cp config.example.json .env.example LICENSE README.MD README.en.md "${stage}/"
  cp -R static/admin "${stage}/static/admin"

  if [[ "$goos" == "windows" ]]; then
    (cd dist && zip -rq "${pkg}.zip" "${pkg}")
  else
    tar -C dist -czf "dist/${pkg}.tar.gz" "${pkg}"
  fi

  rm -rf "$stage"
}

if [[ "${1:-}" == "--build-one" ]]; then
  shift
  build_one "$@"
  exit 0
fi

if [[ "${1:-}" == "--help" || "${1:-}" == "-h" ]]; then
  show_targets
  exit 0
fi

tag="${RELEASE_TAG:-}"
if [[ -z "$tag" && -f VERSION ]]; then
  tag="$(tr -d '[:space:]' < VERSION)"
fi
if [[ -z "$tag" ]]; then
  echo "release tag is empty; set RELEASE_TAG or provide VERSION." >&2
  exit 1
fi

build_version="${BUILD_VERSION:-$tag}"
mkdir -p dist

selected_targets=()
if [[ $# -eq 0 ]]; then
  while IFS= read -r target; do
    selected_targets+=("$target")
  done < <(select_targets)
else
  while IFS= read -r target; do
    selected_targets+=("$target")
  done < <(select_targets "$@")
fi

if [[ ${#selected_targets[@]} -eq 0 ]]; then
  echo "没有选择任何构建目标。" >&2
  exit 1
fi

echo "将构建以下目标："
for target in "${selected_targets[@]}"; do
  read -r _ _ _ label <<< "$target"
  echo "  - $label"
done
echo ""

for target in "${selected_targets[@]}"; do
  read -r goos goarch goarm label <<< "$target"
  build_one "$tag" "$build_version" "$goos" "$goarch" "$goarm" "$label"
done
