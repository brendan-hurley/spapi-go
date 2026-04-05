#!/usr/bin/env bash
# ABOUTME: Pulls the latest SP-API Swagger specs from amzn/selling-partner-api-models
# ABOUTME: into ./models/ and records the upstream commit SHA in models/UPSTREAM.
#
# After running this, review the diff (`git diff models/`), then run
# scripts/generate.sh to regenerate apis/. Commit both together so the
# UPSTREAM file, the spec JSONs, and the generated code always agree.
set -euo pipefail

UPSTREAM_REPO="https://github.com/amzn/selling-partner-api-models.git"
UPSTREAM_REF="${UPSTREAM_REF:-main}"

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
module_root="$(cd "$script_dir/.." && pwd)"
models_dir="$module_root/models"

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

echo ">> cloning $UPSTREAM_REPO (ref: $UPSTREAM_REF) into $tmp_dir"
git clone --depth 1 --branch "$UPSTREAM_REF" "$UPSTREAM_REPO" "$tmp_dir" >/dev/null 2>&1
upstream_sha="$(git -C "$tmp_dir" rev-parse HEAD)"
echo ">> upstream HEAD: $upstream_sha"

if [[ ! -d "$tmp_dir/models" ]]; then
  echo "upstream repo has no models/ directory" >&2
  exit 1
fi

echo ">> replacing ./models"
rm -rf "$models_dir"
mkdir -p "$models_dir"
cp -R "$tmp_dir/models/"* "$models_dir/"

cat > "$models_dir/UPSTREAM" <<EOF
Specs synced from: $UPSTREAM_REPO
Upstream ref:      $UPSTREAM_REF
Upstream commit:   $upstream_sha
Synced at:         $(date -u +'%Y-%m-%dT%H:%M:%SZ')
EOF

echo ">> done. Review changes:"
echo "     git diff --stat models/"
echo "   Then regenerate:"
echo "     bash scripts/generate.sh"
