Draft a "News & Updates" blog post for AfterTouch covering recent git activity, then open a draft PR for review.

## Step 1 — Determine lookback window

Run:
```
git log --format="%ad" --date=short -- docs/content/blog/ | grep -v '_index' | head -1
```

If a date is returned, use it as SINCE.
If the output is empty (no posts yet), compute SINCE = 30 days before today:
- macOS: `date -v-30d +%Y-%m-%d`
- Linux: `date -d '30 days ago' +%Y-%m-%d`

## Step 2 — Collect commits since SINCE

Run:
```
git log --format="%ad %h %s" --date=short --since="$SINCE" --no-merges
```

Exclude these (they are noise):
- Subjects matching: `^(ci|chore|deps|bump|Bump|test|lint|style|code style|debug)`
- Dependabot bumps (subject contains "bump" and includes a package name pattern)
- Routine doc link/URL fixes

Group the remaining commits into categories:
- **NEW FEATURES** — subjects starting with `feat(` or `feat:`
- **BUG FIXES** — subjects starting with `fix(` or `fix:`
- **SECURITY** — subjects starting with `sec` or containing "security", "inject", "path expression"
- **DOCS** — user-visible doc changes only (new guides, major restructures)
- **MAINTENANCE** — everything else that passed the filter

Omit empty categories entirely.

## Step 3 — Current version

Run: `git tag --sort=-version:refname | head -1`

## Step 4 — Determine the period label

Use the first and last commit dates from Step 2 to produce a human-readable label,
e.g. "May 2026" or "April – May 2026".

## Step 5 — Write the blog post

Create the file at: `docs/content/blog/YYYY-MM-slug.md`
- YYYY-MM = today's year-month
- slug = short kebab-case summary of the biggest theme

Use this exact frontmatter shape:
```yaml
---
title: "AfterTouch PERIOD: <one-line theme>"
date: YYYY-MM-DD
description: "<one sentence, ≤200 chars, suitable as a standalone teaser>"
tags:
  - <up to 4 tags from: security, tls, discovery, docs, cli, web, spotify, amazon, health, migration, fixes, ci>
sidebar:
  exclude: true
---
```

Body structure:
1. Opening paragraph (3–5 sentences) explaining what happened and why it matters to someone running AfterTouch.
2. One `##` section per non-empty category. Use bullet points written for an operator audience — no raw git subjects, no internal Go package paths.
3. End with: `**Current release:** vX.Y.Z`

Target length: 300–600 words. Never include real IPs, MAC addresses, account IDs, or device names.

## Step 6 — Create a branch and open a draft PR

```bash
git checkout -b blog/YYYY-MM-update
git add docs/content/blog/YYYY-MM-slug.md
git commit -m "docs(blog): add PERIOD update post"
git push -u origin blog/YYYY-MM-update
gh pr create --draft \
  --title "Blog: PERIOD update post" \
  --body "Automated draft from /blog-update skill. Review content before merging — deployment is automatic on merge to main."
```

If the `documentation` label exists on the repo, add `--label documentation`.

## Step 7 — Done

Report the PR URL. Do not merge, approve, or request review.
