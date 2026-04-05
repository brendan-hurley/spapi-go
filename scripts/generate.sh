#!/usr/bin/env bash
# ABOUTME: Regenerates the per-API Go packages under apis/ from every Swagger
# ABOUTME: 2.0 spec in ./models. Safe to re-run: clears apis/ first.
set -euo pipefail

# ---------- paths ----------
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
module_root="$(cd "$script_dir/.." && pwd)"
models_dir="$module_root/models"
apis_dir="$module_root/apis"

if [[ ! -d "$models_dir" ]]; then
  echo "models dir not found: $models_dir" >&2
  exit 1
fi

# ---------- slugifier ----------
# Lowercase + strip hyphens/underscores/dots. Deliberate: Go package names
# conventionally avoid underscores, and dir names must match package names
# for clean imports. Version suffixes like "_2026-01-01" collapse to
# "20260101", which is unambiguous within each API family.
slugify() {
  local s="$1"
  s="$(echo "$s" | tr '[:upper:]' '[:lower:]')"
  s="${s//[-_.]/}"
  echo "$s"
}

# ---------- clean previous output ----------
echo ">> cleaning $apis_dir"
rm -rf "$apis_dir"
mkdir -p "$apis_dir"

# ---------- generate each spec ----------
count=0
failed=()
while IFS= read -r spec_path; do
  spec_base="$(basename "$spec_path" .json)"
  pkg="$(slugify "$spec_base")"
  out_dir="$apis_dir/$pkg"
  count=$((count + 1))

  printf "[%2d] %-45s -> apis/%s\n" "$count" "$(basename "$spec_path")" "$pkg"

  # Normalize the spec: the Go generator emits one api_<tag>.go file
  # per tag and duplicates operations that carry multiple tags,
  # producing "redeclared in this block" errors at compile time.
  # Collapse every operation to its first tag before generating.
  # (Only vehicles_2024-11-01.json needs this today, but leaving it
  # in for future specs that add multi-tag ops.)
  normalized_spec="/tmp/spapi-spec-$pkg.json"
  python3 -c "
import json
with open('$spec_path') as f: spec = json.load(f)
for methods in spec.get('paths', {}).values():
    for op in methods.values():
        if not isinstance(op, dict): continue
        # Collapse multi-tag ops (see comment in shell script).
        if len(op.get('tags', [])) > 1:
            op['tags'] = op['tags'][:1]
        # Fix type/format contradictions on params. e.g. vendor-direct-
        # fulfillment-orders declares {type:string, format:boolean,
        # default:'true'} which the Go generator emits as
        #   var defaultValue bool = \"true\"
        # (string into bool). Promote these to a real boolean schema.
        for p in op.get('parameters', []) or []:
            if p.get('format') == 'boolean' and p.get('type') == 'string':
                p['type'] = 'boolean'
                if isinstance(p.get('default'), str):
                    p['default'] = p['default'].lower() == 'true'
with open('$normalized_spec', 'w') as f: json.dump(spec, f)"

  # --skip-validate-spec: Amazon's specs contain minor swagger
  # violations (e.g. default values not matching the declared type
  # on array query params). The codegen handles them correctly at
  # runtime; we just need to bypass pre-generation validation.
  if ! npx --yes @openapitools/openapi-generator-cli generate \
      -i "$normalized_spec" \
      -g go \
      -o "$out_dir" \
      --skip-validate-spec \
      --global-property=apiTests=false,modelTests=false,apiDocs=false,modelDocs=false \
      --additional-properties="packageName=$pkg,isGoSubmodule=true,withGoMod=false,generateInterfaces=true,structPrefix=false,enumClassPrefix=true,disallowAdditionalPropertiesIfNotPresent=false" \
      > "/tmp/spapi-gen-$pkg.log" 2>&1; then
    echo "    FAILED (see /tmp/spapi-gen-$pkg.log)"
    failed+=("$spec_base")
    continue
  fi

  # Remove non-Go scaffolding the generator emits that we don't want
  # shipped inside the module (READMEs, travis/git scripts, raw openapi
  # yaml, etc.). Keep *.go plus .openapi-generator-ignore so re-runs
  # don't re-emit deleted files.
  find "$out_dir" -maxdepth 1 -type f \
    ! -name '*.go' ! -name '.openapi-generator-ignore' -delete
  rm -rf "$out_dir/api" "$out_dir/docs" "$out_dir/test" "$out_dir/.openapi-generator"
done < <(find "$models_dir" -name '*.json' | sort)

echo
echo ">> generated $((count - ${#failed[@]}))/$count packages"
if (( ${#failed[@]} > 0 )); then
  echo ">> FAILURES:"
  printf '   - %s\n' "${failed[@]}"
  exit 1
fi
