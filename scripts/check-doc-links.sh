#!/usr/bin/env bash
# Wraps markdown-link-check with a status-code-aware pass/fail policy that
# the tool itself doesn't support: a 404 is a real dead link (fails the
# build), but most other 4xx responses (429 rate-limited, 401/403
# anti-bot/access-restricted, ...) just mean the checker couldn't get a
# clean answer from a site we don't control — those are downgraded to a
# GitHub Actions warning annotation instead of failing CI. Anything else
# (5xx, timeouts, DNS failures) still fails the build, same as before.
#
# Usage: scripts/check-doc-links.sh [root-dir]
# Requires markdown-link-check on PATH and .github/markdown-link-check.json.
set -euo pipefail

root="${1:-.}"
config=".github/markdown-link-check.json"
had_error=0

while IFS= read -r -d '' file; do
    echo "Checking links in $file"

    set +e
    output=$(markdown-link-check -q -v -c "$config" "$file" 2>&1)
    set -e

    echo "$output"

    # markdown-link-check's -v mode logs each dead link more than once
    # (inline with the raw error detail, then again in its summary); de-dupe
    # on (url, code) so each broken link gets one annotation. A plain
    # delimited string, not an associative array (bash 3.2, macOS's
    # default /bin/bash, predates those), with a sentinel on both sides of
    # each key so substring matches can't collide.
    seen="|"

    while IFS= read -r line; do
        [ -z "$line" ] && continue

        url=$(sed -E 's/^[[:space:]]*\[✖\] (.*) → Status: .*/\1/' <<<"$line")
        code=$(sed -E 's/.*Status: ([0-9]+).*/\1/' <<<"$line")

        key="${url}#${code}|"
        case "$seen" in
            *"|${key}"*) continue ;;
        esac
        seen="${seen}${key}"

        if [[ "$code" =~ ^[0-9]+$ ]] && [ "$code" != "404" ] && [ "$code" -ge 400 ] && [ "$code" -lt 500 ]; then
            echo "::warning file=${file}::Link check got HTTP ${code} (treated as non-fatal, see scripts/check-doc-links.sh): ${url}"
        else
            echo "::error file=${file}::Dead link (${code:-no response}): ${url}"
            had_error=1
        fi
    done < <(grep -F '[✖]' <<<"$output" || true)
done < <(find "$root" -name "*.md" -not -path "./tests/*" -not -path "./node_modules/*" -print0)

exit "$had_error"
