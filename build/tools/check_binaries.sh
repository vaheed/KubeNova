#!/usr/bin/env bash
set -euo pipefail

# List potential binary files in repo and fail build if any are found outside tmp dirs.
# If a file is under a tmp-like path, delete it.

shopt -s nullglob
found=()
while IFS= read -r -d '' f; do
  # Skip images and known assets
  case "$f" in
    *.png|*.jpg|*.jpeg|*.gif|*.ico|*.pdf|*.zip) continue ;;
  esac
  # Determine if binary
  if file -b "$f" | grep -qiE 'executable|ELF|Mach-O|PE32'; then
    if [[ "$f" == *tmp* || "$f" == tmp/* || "$f" == */tmp/* ]]; then
      echo "remove tmp binary: $f" >&2
      rm -f "$f"
      continue
    fi
    found+=("$f")
  fi
done < <(git ls-files -z)

if (( ${#found[@]} > 0 )); then
  echo "Binary files found in repository (not allowed):" >&2
  printf ' - %s\n' "${found[@]}" >&2
  exit 1
fi
echo "No stray binaries."

