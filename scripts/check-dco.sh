#!/usr/bin/env bash
# Check the Developer Certificate of Origin (DCO) sign-off of every commit in a
# range. Used by .github/workflows/dco.yml and usable locally:
#
#   scripts/check-dco.sh origin/main HEAD
#
# A commit passes when one of its trailer lines is
#   Signed-off-by: Name <email>
# with the same email as the commit author. Commits authored by bots are
# skipped, and merge commits are not checked.
set -euo pipefail

base="${1:-}"
head="${2:-HEAD}"

if [[ -z "$base" ]]; then
    echo "usage: $0 <base-rev> [head-rev]" >&2
    exit 2
fi

failed=0
checked=0

while read -r sha; do
    [[ -z "$sha" ]] && continue

    author_name="$(git show -s --format='%an' "$sha")"
    author_email="$(git show -s --format='%ae' "$sha")"
    subject="$(git show -s --format='%s' "$sha")"

    # Bot commits are exempt: they cannot sign a certificate.
    if [[ "$author_name" == *"[bot]"* ]] || [[ "$author_email" == *"[bot]"* ]] ||
        [[ "$author_name" == "dependabot" ]] || [[ "$author_email" == "noreply@github.com" ]]; then
        echo "skip  ${sha:0:12} ${subject} (bot commit)"
        continue
    fi

    checked=$((checked + 1))
    if git show -s --format='%b' "$sha" |
        grep -i '^Signed-off-by:' |
        sed -E 's/^[^<]*<([^>]*)>.*/\1/' |
        grep -qiFx "$author_email"; then
        echo "ok    ${sha:0:12} ${subject}"
    else
        echo "error ${sha:0:12} ${subject}"
        echo "      missing or mismatched Signed-off-by: ${author_name} <${author_email}>"
        failed=$((failed + 1))
    fi
done < <(git rev-list --no-merges "${base}..${head}")

if [[ "$checked" -eq 0 ]]; then
    echo "No commits to check between ${base} and ${head}."
    exit 0
fi

if [[ "$failed" -ne 0 ]]; then
    echo
    echo "$failed commit(s) without a matching Signed-off-by line."
    echo "Amend them with 'git commit --amend -s', or rebase with"
    echo "'git rebase --signoff ${base}' and force-push your own branch."
    exit 1
fi

echo "All $checked commit(s) carry a Signed-off-by line that matches the author."
