#!/usr/bin/env bash
# Gamaj brand guard: the raster favicon set is generated, never hand-edited.
#
# Every repository that ships the brand mark carries its own copy of
# `tools/render-brand-assets.mjs` (the exact same renderer as Web) plus the
# committed PNG/ICO set in the directory that holds `brand-assets.sha256`.
# This script regenerates the rasters from the in-repo tool and fails when the
# committed set has drifted from it, so the icons can never silently diverge
# from the canonical Web assets.
#
# Run locally: bash scripts/ci/brand-assets-check.sh

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

TOOL="tools/render-brand-assets.mjs"

if [ ! -f "$TOOL" ]; then
  printf 'brand-assets-check: ERROR: missing render tool %s\n' "$TOOL" >&2
  exit 1
fi
if ! command -v node >/dev/null 2>&1; then
  printf 'brand-assets-check: ERROR: node is required to regenerate the brand assets\n' >&2
  exit 1
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
node "$TOOL" "$tmp" >/dev/null

failed=0
found=0
while IFS= read -r manifest; do
  [ -n "$manifest" ] || continue
  found=1
  dir="$(dirname "$manifest")"

  # 1. Every regenerated raster must exist in the brand directory unchanged.
  for generated in "$tmp"/*; do
    name="$(basename "$generated")"
    if [ ! -f "$dir/$name" ]; then
      printf 'brand-assets-check: ERROR: %s/%s is missing\n' "$dir" "$name" >&2
      failed=1
      continue
    fi
    if ! cmp -s "$generated" "$dir/$name"; then
      printf 'brand-assets-check: ERROR: %s/%s drifted from %s output\n' "$dir" "$name" "$TOOL" >&2
      failed=1
    fi
  done

  # 2. Every raster the manifest promises must be one this tool produces.
  while IFS= read -r entry; do
    [ -n "$entry" ] || continue
    name="$(printf '%s\n' "$entry" | awk '{print $NF}')"
    name="${name#\*}"
    case "$name" in
      *.webmanifest|*.xml) continue ;;
    esac
    if [ ! -f "$tmp/$name" ]; then
      printf 'brand-assets-check: ERROR: %s/%s is not produced by %s\n' "$dir" "$name" "$TOOL" >&2
      failed=1
    fi
  done < "$manifest"
done < <(find . -name 'brand-assets.sha256' \
  -not -path './node_modules/*' -not -path './.git/*' 2>/dev/null)

if [ "$found" -eq 0 ]; then
  printf 'brand-assets-check: ERROR: no brand-assets.sha256 manifest found\n' >&2
  exit 1
fi

if [ "$failed" -ne 0 ]; then
  printf 'brand-assets-check: FAILED\n' >&2
  exit 1
fi
printf 'brand-assets-check: OK\n'
