#!/usr/bin/env bash
# Gamaj CI guard: keeps the repository honest.
#
# Fails when a workflow file references:
#   1. a workflow that does not exist in this repository,
#   2. a branch other than the Gamaj branch (`Asli`) in trigger filters,
#      or a foreign branch of a Gamaj repository in any URL,
#   3. Docker or GHCR in any form.
# It also re-verifies the shared brand assets against the committed
# `brand-assets.sha256` manifest, so the raster identity cannot drift.
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

if [ "$failed" -ne 0 ]; then
  printf 'workflow-guard: FAILED\n' >&2
  exit 1
fi
printf 'workflow-guard: OK\n'
