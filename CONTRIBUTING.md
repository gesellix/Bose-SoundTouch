# Contributing to AfterTouch

Thank you for your interest in contributing to **AfterTouch**!

AfterTouch is a community-built toolkit that keeps Bose SoundTouch speakers
usable after Bose shut down the SoundTouch cloud. It is a Go codebase that ships
several tools plus a reusable library:

- **soundtouch-service** the local cloud replacement (emulates `streaming.bose.com` and the `bmx` services)
- **soundtouch-cli** command-line control of one or more speakers
- **soundtouch-player** the web UI for radio browsing and device control
- **soundtouch-backup** on-device backup and restore helper
- **pkg/** the underlying Go library (HTTP + WebSocket client, models, discovery, ...)

We are an open community: we both provide and ask for support. Contributions of
every size are welcome, and you do not need to be a Go developer to help.

## Ways to contribute

- **Bug reports** even a clear reproducer is a real contribution. An attached
  diagnostic report (see [Reporting issues](#reporting-issues)) helps enormously.
- **Device compatibility reports** tell us how AfterTouch behaves with your speaker model.
- **Code** bug fixes, features, refactoring, tests, tooling.
- **Documentation** guides, examples, troubleshooting notes, inline doc comments.
- **Helping others** answering questions in [Discussions](https://github.com/gesellix/Bose-SoundTouch/discussions).
- **Donations** if AfterTouch kept a speaker (or several) of yours alive and you
  want to give back, [GitHub Sponsors](https://github.com/sponsors/gesellix) is
  open. There is no expectation, and everything here stays MIT regardless.

By submitting a code or documentation contribution you agree to license it under
the project's [MIT License](LICENSE).

## Code of Conduct

This project follows a [Code of Conduct](CODE_OF_CONDUCT.md). By participating,
you agree to uphold it. Please report unacceptable behavior to the maintainer.

## Getting started

### Prerequisites

- **Go** (version per [`go.mod`](go.mod), currently the 1.26.x series)
- **Git**
- **Make** (recommended; drives builds and the quality gate)
- **Docker** (only needed for the HTTP-client integration tests)
- A **SoundTouch device** is optional but valuable for testing

### Build and run

```bash
# Clone your fork
git clone https://github.com/YOUR-USERNAME/Bose-SoundTouch.git
cd Bose-SoundTouch

# Build all binaries into ./build/
make build

# Try the CLI
./build/soundtouch-cli --help

# Run the local service on port 8000
make dev-service
```

Other useful targets: `make build-cli`, `make build-service`, `make build-player`,
`make dev-discover` (find devices on the LAN). See the `Makefile` for the full list.

## Development workflow

1. For anything non-trivial, **open an issue first** so we can agree on the approach.
2. Create a feature branch from `main`.
3. Make small, focused changes with tests.
4. Run the quality gate before pushing:
   ```bash
   make check   # fmt + vet + tests (+ the Docker-based HTTP-client integration suite)
   make lint    # golangci-lint, must be clean
   ```
   If you do not have Docker handy, run `make test` and `make lint` and say so in
   the PR; CI runs the full gate on every PR.
5. Open a pull request. The PR template walks you through what to include.

New to the codebase? **[`CLAUDE.md`](CLAUDE.md)** is the entry point for any
session (human or AI): it explains the layout, build/test commands, and the
load-bearing gotchas. Please skim it before larger changes.

### A note on AI-assisted contributions

AI and agent-assisted code is welcome, we use it here too. What we cannot accept
is unreviewed "slop": large generated diffs the author has not read, run, or
understood. Keep PRs small and focused, make sure `make check` passes, and be
ready to explain your changes during review.

### Never commit personal or device data

This repository is public. Do not commit real LAN IPs, MAC addresses, device IDs,
Bose account IDs, tokens, firmware binaries, or Wi-Fi credentials, in code, tests,
fixtures, commit messages, or PR text. Use the RFC-5737 documentation ranges
(`192.0.2.0/24`, `198.51.100.0/24`, `203.0.113.0/24`) and placeholder identifiers
in examples. See [`CLAUDE.md`](CLAUDE.md) for the full list.

## Coding and testing guidelines

- **Follow standard Go style:** `gofmt`, `go vet`, and `golangci-lint` all clean.
  `golangci-lint run --fix` auto-fixes some issues.
- **Tests are expected** with every change. Prefer unit tests with `httptest`
  mocks; use integration tests where a unit test is impractical.
- **Use real device data for fixtures where possible,** anonymized per the rule
  above. Reproducer tests should graduate into permanent regression or
  documentation tests rather than being deleted.
- **Wrap errors with context** (`fmt.Errorf("...: %w", err)`) and validate inputs
  with helpful messages.
- **Keep it simple.** Favor readable, self-explanatory code over cleverness.
- **The SoundTouch Web API is XML on the wire;** internal service-to-service
  messages are JSON. (One sharp edge: the `ETag` response header must keep its
  exact capitalization, see `CLAUDE.md`.)

## Reporting issues

Open a [new issue](https://github.com/gesellix/Bose-SoundTouch/issues) and pick
one of the forms; they keep reports easy to triage:

- **Bug report** something in AfterTouch is not working as it should
- **Feature request** an idea or improvement
- **Device compatibility report** how AfterTouch behaves with your speaker model

For bugs, the most helpful thing you can attach is an **encrypted diagnostic
report**. In the AfterTouch admin UI, open the **Health tab** and click
**Download diagnostic report**. The file is encrypted to the maintainer's key, so
only the maintainer can open it.

To share it, the Health tab recommends **email**: send it to
<aftertouch-support@gesellix.net>. You can also attach it to a GitHub issue, but
GitHub blocks `.age` uploads, so rename the file to `.age.txt` (or zip it) first.

To be transparent about what it holds: the structured summary (`diagnostic.json`)
and `settings.json` have credentials and OAuth secrets redacted, but the raw
datastore files (for example `Sources.xml` and `full.xml`) are included **as-is**.
For TuneIn, Radio Browser, and Local Internet Radio those carry only
AfterTouch-generated placeholders, but for **linked accounts such as Spotify or
Amazon they can contain the access tokens your speaker uses**. There is currently
no setting that redacts the datastore files. If that is a concern, unlink those
services before exporting, or email the report privately rather than attaching it
to a public issue.

Before filing, the
[Troubleshooting Guide](https://gesellix.github.io/Bose-SoundTouch/docs/guides/TROUBLESHOOTING/)
often has the answer. For "how do I...?" questions, please use
[Discussions](https://github.com/gesellix/Bose-SoundTouch/discussions) rather than
the issue tracker.

### Security issues

Please do not open a public issue for a security vulnerability. Report it
privately to the maintainer (GitHub's private vulnerability reporting on the
repository's Security tab is the preferred channel) and allow reasonable time for
a fix before any public disclosure.

## Community

- **Discussions:** questions, ideas, and general support
- **Issues:** bugs, feature requests, compatibility reports
- **Pull requests:** code and documentation

Please be patient and respectful in all interactions. Significant contributions
are credited in release notes.

## Support the project

[![GitHub Sponsors](https://img.shields.io/github/sponsors/gesellix?label=Sponsor%20on%20GitHub&logo=GitHub&color=ea4aaa)](https://github.com/sponsors/gesellix)

Sponsorship is entirely optional. Code, docs, bug reports, and helping others
remain the most useful contributions.

## Resources

- [AfterTouch documentation](https://gesellix.github.io/Bose-SoundTouch/)
- [Survival Guide](https://gesellix.github.io/Bose-SoundTouch/docs/guides/SURVIVAL-GUIDE/)
- [API Cookbook](https://gesellix.github.io/Bose-SoundTouch/docs/reference/API-COOKBOOK/)
- [API Endpoints](https://gesellix.github.io/Bose-SoundTouch/docs/reference/API-ENDPOINTS/)
- [Go Documentation](https://golang.org/doc/) and [Effective Go](https://golang.org/doc/effective_go.html)

---

**Thank you for contributing!** Every contribution helps keep the SoundTouch
community's speakers playing.
