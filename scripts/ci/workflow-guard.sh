#!/usr/bin/env bash
# Gamaj CI guard: keeps the repository honest.
#
# Fails when a workflow file references:
#   1. a workflow that does not exist in this repository,
#   2. a branch other than the Gamaj branch (`Asli`) in trigger filters,
#      or a foreign branch of a Gamaj repository in any URL,
#   3. Docker or GHCR in any form,
#   4. a release tag filter that is not the `is.*` glob or a concrete
#      `is.<major>.<minor>.<patch>` tag.
# It also re-verifies the shared brand assets against the committed
# `brand-assets.sha256` manifest, and checks that the release asset names a
# repository builds agree with the names its installers request, so neither the
# raster identity nor the release contract can drift unnoticed.
#
# Run locally: bash scripts/ci/workflow-guard.sh

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

WF_DIR=".github/workflows"
failed=0

note() { printf '  %s\n' "$*" >&2; }
fail_with() { printf 'workflow-guard: ERROR: %s\n' "$*" >&2; failed=1; }

shopt -s nullglob
WF_FILES=("$WF_DIR"/*.yml "$WF_DIR"/*.yaml)
shopt -u nullglob

if [ "${#WF_FILES[@]}" -eq 0 ]; then
  note "no workflow files found under $WF_DIR"
fi

# ---------------------------------------------------------------------------
# 1. Every referenced workflow name must exist in this repository.
# ---------------------------------------------------------------------------
defined_names="$(
  grep -h '^name:' "${WF_FILES[@]}" 2>/dev/null \
    | sed -e 's/^name:[[:space:]]*//' -e 's/^["'\'']//' -e 's/["'\'']$//' \
    | tr -d '\r' | sort -u || true
)"

referenced=0
for wf in "${WF_FILES[@]}"; do
  while IFS= read -r ref; do
    [ -n "$ref" ] || continue
    referenced=1
    if ! printf '%s\n' "$defined_names" | grep -Fxq "$ref"; then
      fail_with "$wf references a workflow that does not exist: '$ref'"
    fi
  done < <(awk '
    /^  workflow_run:/ { in_wr = 1; next }
    in_wr && /^[[:space:]]+workflows:/ { in_list = 1; next }
    in_list && /^[[:space:]]+- / {
      sub(/^[[:space:]]+- /, ""); gsub(/["\047]/, ""); print; next
    }
    in_list { in_list = 0 }
  ' "$wf" | tr -d '\r')
done
[ "$referenced" -eq 1 ] || note "no workflow_run references to check"

# ---------------------------------------------------------------------------
# 2a. Trigger branch filters must only use the Gamaj branch.
# ---------------------------------------------------------------------------
for wf in "${WF_FILES[@]}"; do
  while IFS= read -r branch; do
    [ -n "$branch" ] || continue
    case "$branch" in
      Asli|'*'|'**') ;;
      *) fail_with "$wf triggers on foreign branch '$branch'; only 'Asli' is allowed" ;;
    esac
  done < <(awk '
    /^on:/ { in_on = 1; next }
    in_on && /^[^[:space:]]/ { in_on = 0 }
    in_on && /^[[:space:]]+branches:/ { in_branches = 1; next }
    in_branches && /^[[:space:]]+- / {
      sub(/^[[:space:]]+- /, ""); gsub(/["\047]/, ""); print; next
    }
    in_branches { in_branches = 0 }
  ' "$wf" | tr -d '\r')
done

# ---------------------------------------------------------------------------
# 2b. No foreign branch of a Gamaj repository anywhere in the tree.
# ---------------------------------------------------------------------------
foreign_re='TheGamaj/(Panel|Node|Bot)/(dev|master|main)([/?"'\''[:space:]]|$)'
while IFS= read -r hit; do
  [ -n "$hit" ] || continue
  fail_with "foreign Gamaj branch reference: $hit"
done < <(grep -rEn "$foreign_re" \
  --include='*.go' --include='*.md' --include='*.yml' --include='*.yaml' \
  --include='*.sh' --include='*.ts' --include='*.tsx' --include='*.js' \
  --include='*.mjs' --include='*.json' \
  --exclude-dir=node_modules --exclude-dir=.git --exclude-dir=dist \
  --exclude-dir=build --exclude-dir=data \
  --exclude='workflow-guard.sh' . 2>/dev/null || true)

# ---------------------------------------------------------------------------
# 3. No Docker / GHCR.
# ---------------------------------------------------------------------------
if [ "${#WF_FILES[@]}" -gt 0 ]; then
  while IFS= read -r hit; do
    [ -n "$hit" ] || continue
    fail_with "Docker/GHCR reference: $hit"
  done < <(grep -rEin 'ghcr|docker' "${WF_FILES[@]}" 2>/dev/null || true)
fi

# ---------------------------------------------------------------------------
# 2c. Release tag filters must follow the Gamaj `is.X.Y.Z` scheme.
# ---------------------------------------------------------------------------
for wf in "${WF_FILES[@]}"; do
  while IFS= read -r tag; do
    [ -n "$tag" ] || continue
    if [ "$tag" = "is.*" ]; then
      continue
    fi
    if [[ "$tag" =~ ^is\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
      continue
    fi
    fail_with "$wf has a malformed release tag filter '$tag'; only 'is.*' or 'is.<major>.<minor>.<patch>' are allowed"
  done < <(awk '
    /^on:/ { in_on = 1; next }
    in_on && /^[^[:space:]]/ { in_on = 0 }
    in_on && /^[[:space:]]+tags:/ { in_tags = 1; next }
    in_tags && /^[[:space:]]+- / {
      sub(/^[[:space:]]+- /, ""); gsub(/["\047]/, ""); print; next
    }
    in_tags { in_tags = 0 }
  ' "$wf" | tr -d '\r')
done

# ---------------------------------------------------------------------------
# 4. Shared brand assets must match the committed hash manifest.
# ---------------------------------------------------------------------------
while IFS= read -r manifest; do
  [ -n "$manifest" ] || continue
  dir="$(dirname "$manifest")"
  base="$(basename "$manifest")"
  if (cd "$dir" && sha256sum -c "$base" >/dev/null 2>&1); then
    note "brand assets verified: $dir"
  else
    fail_with "brand assets drifted from $base in $dir"
  fi
done < <(find . -name 'brand-assets.sha256' \
  -not -path './node_modules/*' -not -path './.git/*' 2>/dev/null)

# ---------------------------------------------------------------------------
# 4b. Regenerate the brand rasters from the in-repo render tool and confirm
#     they still match the committed files, so the renderer and the assets
#     cannot drift apart.
# ---------------------------------------------------------------------------
if [ -f scripts/ci/brand-assets-check.sh ]; then
  if bash scripts/ci/brand-assets-check.sh >/dev/null 2>&1; then
    note "brand rasters regenerate cleanly"
  else
    fail_with "brand rasters drifted from tools/render-brand-assets.mjs; run 'bash scripts/ci/brand-assets-check.sh' for details"
  fi
else
  fail_with "missing scripts/ci/brand-assets-check.sh"
fi

# ---------------------------------------------------------------------------
# 5. Release asset names must agree between the workflows and the installers.
#
# Each repository builds one primary Linux release asset that its installer
# downloads. Normalise the version/architecture placeholders on both sides and
# require the exact same shape, so renaming an asset on one side only is caught
# here instead of at install time.
# ---------------------------------------------------------------------------
module_name="$(awk 'NR==1{print $2}' go.mod 2>/dev/null | tr -d '\r' || true)"
asset_workflow=""
asset_installer=""
asset_shape=""
asset_label=""
case "$module_name" in
  github.com/TheGamaj/Panel)
    asset_workflow=".github/workflows/binary-build.yml"
    asset_installer="scripts/gamaj/gamaj.sh"
    asset_shape='gamaj-linux-[A-Za-z0-9_.@-]+\.tar\.gz'
    asset_label="Panel linux package"
    ;;
  github.com/TheGamaj/Node)
    asset_workflow=".github/workflows/binary-build.yml"
    asset_installer="scripts/gamaj/gamaj-node.sh"
    asset_shape='gamaj-node-[A-Za-z0-9_.@-]+-linux-[A-Za-z0-9_.@-]+'
    asset_label="Node linux binary"
    ;;
  github.com/TheGamaj/Bot)
    asset_workflow=".github/workflows/verify.yml"
    asset_installer="scripts/install.sh"
    asset_shape='gamaj-bot-linux-[A-Za-z0-9_.@-]+'
    asset_label="Bot linux binary"
    ;;
  *)
    note "no release asset contract declared for module '${module_name:-unknown}'"
    ;;
esac

normalize_assets() {
  # Collapse build-time placeholders and architecture tokens to `@`, strip
  # quotes, then list the distinct `gamaj-...` asset names.
  sed -E \
    -e 's/\$\{\{[^}]*\}\}/@/g' \
    -e 's/\$\{[^}]*\}/@/g' \
    -e 's/\$[A-Za-z_][A-Za-z0-9_]*/@/g' \
    -e 's/["'\'']//g' \
    -e 's/(^|[^A-Za-z0-9])(386|amd64|arm64|armv5|armv6|armv7|s390x|x86_64|aarch64)([^A-Za-z0-9]|$)/\1@\3/g' \
    -e 's/\.sha256$//' \
    "$1" 2>/dev/null | grep -oE 'gamaj-[A-Za-z0-9_.@-]+' | sort -u || true
}

if [ -n "$asset_workflow" ]; then
  if [ ! -f "$asset_workflow" ]; then
    fail_with "missing build workflow for the $asset_label contract: $asset_workflow"
  fi
  if [ ! -f "$asset_installer" ]; then
    fail_with "missing installer for the $asset_label contract: $asset_installer"
  fi
  if [ -f "$asset_workflow" ] && [ -f "$asset_installer" ]; then
    if ! normalize_assets "$asset_workflow" | grep -Eq "$asset_shape"; then
      fail_with "$asset_workflow no longer builds a $asset_label matching '$asset_shape'"
    fi
    if ! normalize_assets "$asset_installer" | grep -Eq "$asset_shape"; then
      fail_with "$asset_installer downloads a $asset_label that drifted from '$asset_shape'"
    fi
    note "release asset verified: $asset_label"
  fi
fi

if [ "$failed" -ne 0 ]; then
  printf 'workflow-guard: FAILED\n' >&2
  exit 1
fi
printf 'workflow-guard: OK\n'
