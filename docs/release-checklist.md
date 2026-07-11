# v0.1.0-alpha Release Checklist

Use this checklist for the actual alpha release. Do not claim private real-export
testing until a maintainer has completed it.

## Automated Checks

- [ ] Formatting verification passes.
- [ ] `go test -count=1 ./...` passes.
- [ ] `go vet ./...` passes.
- [ ] Supported build matrix compiles: Windows amd64, Linux amd64/arm64, macOS
  amd64/arm64.
- [ ] Archive contents contain only intended release files.
- [ ] SHA-256 checksums generate and verify for all five archives.
- [ ] Network, browser-automation, credential/secret-persistence, and
  mutation-boundary safety checks pass.
- [ ] Tag-push workflow creates a draft release with five archives and
  `checksums.txt`.

## Maintainer Smoke Tests

- [ ] Download generated Windows archive from draft release.
- [ ] Verify its checksum.
- [ ] Launch packaged Windows binary.
- [ ] Launch packaged Linux or macOS binary where a suitable environment exists.
- [ ] Demo import.
- [ ] Real Instagram export import.
- [ ] Partial export import.
- [ ] Large export import.
- [ ] Review and filtering.
- [ ] Plan generation.
- [ ] Unfollow manual action.
- [ ] Unlike manual action.
- [ ] Delete-comment manual action.
- [ ] Stop cleanup.
- [ ] Close Vanish.
- [ ] Reopen Vanish.
- [ ] Resume cleanup.
- [ ] Complete cleanup.
- [ ] Return to original plan.
- [ ] Local-data wipe.
- [ ] Confirm external ZIP and plan files remain untouched.

## Final Release Steps

- [ ] Merge release-prep PR.
- [ ] Update local `main`.
- [ ] Confirm `v0.1.0-alpha` does not already exist on the target remote.
- [ ] Create annotated tag `v0.1.0-alpha`.
- [ ] Push tag.
- [ ] Wait for release workflow.
- [ ] Open generated draft GitHub Release.
- [ ] Download and inspect artifacts.
- [ ] Verify `checksums.txt`.
- [ ] Edit title and release notes where needed.
- [ ] Set prerelease classification manually if desired.
- [ ] Publish manually.

The workflow never runs for pull requests, branch pushes, workflow-only changes,
or local builds. It creates a draft only after a pushed `v*` tag passes release
validation and all packaging checks.
