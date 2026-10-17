#!/bin/bash
# The docs part of the static rung (spec §9.5 rung 1): every markdown file under docs/ has the front matter
# the docs standard asks for (title, a status and a date; style and related are optional), a plain file name, and links that point at files that exist. Prose rules
# (Vale) wait for a later change. Run in a leaf's top folder; it reads and changes nothing.
set -euo pipefail
[ -d docs ] || { echo "docs: no docs/ folder, nothing to check"; exit 0; }
problems=0
say() { echo "docs: $*"; problems=$((problems + 1)); }
while IFS= read -r -d '' f; do
  name="$(basename "$f")"
  [[ "$name" =~ ^[a-z0-9][a-z0-9._-]*\.md$ ]] || say "$f: the file name must be lower case words joined by dashes, ending in .md"
  if [ "$(head -n 1 "$f")" != "---" ]; then
    say "$f: starts without front matter (--- on the first line)"
  else
    head_block="$(awk 'NR == 1 { next } /^---$/ { exit } { print }' "$f")"
    for key in title status date; do
      grep -q "^$key:" <<<"$head_block" || say "$f: the front matter has no $key"
    done
    status="$(sed -n 's/^status: *//p' <<<"$head_block" | head -n 1)"
    [ -z "$status" ] || [[ "$status" =~ ^(draft|accepted|implemented|superseded)$ ]] || say "$f: status is $status; it is draft, accepted, implemented or superseded"
    date="$(sed -n 's/^date: *//p' <<<"$head_block" | head -n 1)"
    [ -z "$date" ] || [[ "$date" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]] || say "$f: date is $date; it is YYYY-MM-DD"
  fi
  dir="$(dirname "$f")"
  while IFS= read -r link; do
    target="${link%%#*}"
    case "$target" in '' | http://* | https://* | mailto:*) continue ;; esac
    [ -e "$dir/$target" ] || say "$f: the link $link points at nothing"
  done < <(grep -o ']([^)]*)' "$f" | sed -e 's/^](//' -e 's/)$//' || true)
done < <(find docs -type f -name '*.md' -print0 | sort -z)
[ "$problems" -eq 0 ] || { echo "docs: $problems problem(s)"; exit 1; }
echo "docs: ok"
