Draft a "News & Updates" blog post for AfterTouch covering recent git activity, commit it to a branch, and hand the maintainer the commands to push and open a draft PR (the maintainer pushes, not you).

## Step 1 — Determine lookback window

If the invocation arguments name an explicit starting point (a tag like `v0.93.1` or a
date), use that as SINCE. For a tag, resolve its date:
`git log -1 --format=%ad --date=short <tag>`. An explicit argument always overrides the
auto-detection below.

Otherwise, auto-detect from the last published post:
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
2. The body. Prefer a narrative that ties the changes into a story (what shifted, why it matters), not a bare aggregation of the release notes. Group related work under `##` sections (the commit categories are raw material, not the final headings). Write for an operator audience: no raw git subjects, no internal Go package paths. A short bullet list inside a section is fine, but the post should read like prose, not a changelog dump.
3. Close with the standard footer convention used by the existing posts, so every post ends the same way:

   ```markdown
   ## Current release

   **vX.Y.Z**, released MONTH D, YYYY

   This blog will be updated monthly, or whenever something significant ships.
   Subscribe to the [GitHub releases](https://github.com/gesellix/Bose-SoundTouch/releases)
   for individual version notes.
   ```

   Get the release date with `git log -1 --format=%ad --date=format:'%B %-d, %Y' vX.Y.Z`.
   When in doubt about any recurring element (footer, release line, tags), match the most
   recent existing post under `docs/content/blog/` rather than inventing a new convention.
   Never retrofit or restyle already-published posts to fit a new convention — they are
   dated records; a new convention applies going forward only.

Target length: 300–600 words (longer is fine when the story warrants it). Never include
real IPs, MAC addresses, account IDs, or device names.

**No em dashes.** Do not use the em dash character (`—`) anywhere in the post; use commas,
parentheses, colons, or separate sentences. (En dashes in a period label like
`April – May 2026` are fine.) Verify with `grep -c '—' <file>` before committing.

## Step 6 — Create a branch and commit (do NOT push)

```bash
git checkout -b blog/YYYY-MM-update
git add docs/content/blog/YYYY-MM-slug.md
git commit -m "docs(blog): add PERIOD update post"
```

**Do not push and do not open the PR yourself.** The maintainer always pushes over SSH
(see the global and project instructions). Pushing on their behalf, including over HTTPS
with a token or by switching the remote, is not allowed.

## Step 7 — Done

Hand the maintainer the ready-to-run commands to push and open the draft PR, then stop:

```bash
git push -u origin blog/YYYY-MM-update
gh pr create --draft \
  --title "Blog: PERIOD update post" \
  --body "Update post covering recent changes. Review content before merging — deployment is automatic on merge to main."
```

If the `documentation` label exists on the repo, add `--label documentation`.

Do not merge, approve, or request review.
