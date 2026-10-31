# PipelineFixer Journal

## 2025-05-18 - Linux Static Build & Go CI Verification Constraints

**Learning:** The IBM FlashSystem Collector target architecture requires static CGO-disabled Linux AMD64 binary compilation (`CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o flashsystem-collector .`). The standard `go.yml` workflow was only running `go build -v ./...` without verifying dependency integrity (`go mod verify`), static analysis (`go vet ./...`), or cross-compilation for Linux AMD64 binaries. Additionally, `golangci-lint` binary builds in local runner environments might mismatch when Go is updated to 1.25+ unless properly aligned.

**Action:** Always ensure `.github/workflows/go.yml` includes Go module caching (`cache: true`), dependency verification (`go mod verify`), static code check (`go vet`), and explicit target Linux AMD64 static binary compilation.

## 2025-05-18 - Zizmor Security Audit Findings & Action Pinning

**Learning:** `zizmor` flags security vulnerabilities in GitHub Actions workflows, specifically `artipacked` (credential persistence on disk from `actions/checkout` when `persist-credentials: false` is not set) and `unpinned-uses` (unpinned actions not pinned to commit SHAs). Rather than disabling or ignoring checks in configuration, fixing the workflows directly by pinning actions to commit SHAs and adding `persist-credentials: false` allows `zizmor` to pass cleanly with exit code 0.

**Action:** Always pin GitHub Actions to full commit SHAs with semantic version comments and specify `persist-credentials: false` on `actions/checkout` across workflow files.
