---
name: release-cli
description: Use when cutting a tagged release of the bw CLI from main, watching the goreleaser + bump-formula pipeline, and confirming the Homebrew tap PR opens and passes strict audit.
---

# Release the bw CLI

Tag-driven release. Pushing `vX.Y.Z` triggers `.github/workflows/release.yml`, which runs three sequential jobs:

1. `test` — runs `go test ./...` on linux/mac/windows
2. `release` — goreleaser builds binaries, creates the GitHub release, pushes Docker images
3. `bump-formula` — `mislav/bump-homebrew-formula-action@v3` opens a PR on `Bandwidth/homebrew-tap` updating `url` + `sha256` to the new tag's source archive

The tap formula is **source-build**: `brew install` runs `go build` (~22s, vs. ~5s on the old pre-built tarball flow). The formula structure lives canonically in the tap repo — the action only bumps version/sha.

## Pre-flight (on the cli repo)

1. Confirm `gh auth status` is on the **kshahbw** account (Bandwidth writes need it).
2. On `main`, clean tree, pulled: `git checkout main && git pull && git status`.
3. Last release: `git describe --tags --abbrev=0`.
4. Decide next version (semver against the changes since that tag).

## Cut the tag

5. `git tag -a vX.Y.Z -m "Release vX.Y.Z"`
6. `git push origin vX.Y.Z`

## Watch the cli release pipeline

7. `gh run watch` (or `gh run list --workflow=release.yml --limit 1`) — wait for the workflow to finish. All three jobs (`test`, `release`, `bump-formula`) need to go green.
8. Confirm the GitHub release exists: `gh release view vX.Y.Z`.

## Watch the homebrew-tap PR

`bump-formula` opens a PR on `Bandwidth/homebrew-tap` with title `band vX.Y.Z`. The tap repo's `ci.yml` runs `brew audit --strict band` (the `CI / audit` check) on every PR — don't run it locally.

9. Find it: `gh pr list --repo Bandwidth/homebrew-tap --search "band vX.Y.Z in:title" --state open`.
10. Wait for the `audit` check to go green: `gh pr checks <pr-number> --repo Bandwidth/homebrew-tap --watch`.
11. Merge it: `gh pr merge <pr-number> --repo Bandwidth/homebrew-tap --squash`.

## Smoke test

12. `brew update && brew upgrade band && band version` — confirm the new version installs and prints. First-time install on a clean machine takes ~22s (Go compile); upgrade of an existing install is faster.

## If something fails

- **Tests fail in release workflow** → the tag is already pushed; delete it (`git push --delete origin vX.Y.Z && git tag -d vX.Y.Z`), fix on a branch, re-tag.
- **`release` (goreleaser) fails** → check `gh run view --log-failed` on the release run; usually a missing secret or changelog filter issue.
- **`bump-formula` fails** → most often a `HOMEBREW_TAP_TOKEN` scope/expiry issue or a rate limit on the tap repo; the GitHub release still went out, so fix the token and re-run just that job (`gh run rerun <run-id> --job bump-formula`).
- **Tap audit fails strict** → fix `Formula/band.rb` directly in the tap repo and merge that as a separate PR; the bump-formula PR can be rebased after. The cli repo no longer owns the formula structure, so don't try to fix it from there.
- **Repro audit locally** before re-tagging: clone the tap repo, then `brew audit --strict band`.
