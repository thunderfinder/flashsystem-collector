# PipelineFixer Journal

## 2025-05-18 - Linux Static Build & Go CI Verification Constraints

**Learning:** The IBM FlashSystem Collector target architecture requires static CGO-disabled Linux AMD64 binary compilation (`CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o flashsystem-collector .`). The standard `go.yml` workflow was only running `go build -v ./...` without verifying dependency integrity (`go mod verify`), static analysis (`go vet ./...`), or cross-compilation for Linux AMD64 binaries. Additionally, `golangci-lint` binary builds in local runner environments might mismatch when Go is updated to 1.25+ unless properly aligned.

**Action:** Always ensure `.github/workflows/go.yml` includes Go module caching (`cache: true`), dependency verification (`go mod verify`), static code check (`go vet`), and explicit target Linux AMD64 static binary compilation.

## 2025-05-18 - Optional Secret Guards & Security Integration Placeholders

**Learning:** Optional third-party integration workflows like `sonarqube.yml`, `endorlabs.yml`, `veracode.yml`, and `lint.yml` fail when required secrets or configuration variables (such as `SONAR_TOKEN`, `ENDOR_NAMESPACE`, `VERACODE_API_ID`) are missing or left as default placeholders (e.g. `namespace: "example"`). Using prebuilt `golangci-lint` binaries built on Go 1.24 also fails when target `go.mod` specifies Go 1.25+.

**Action:** Guard third-party security workflows with `if: env.<SECRET_VAR> != ''` or `if: vars.<CONFIG_VAR> != ''` at job or step levels so CI jobs pass when optional external services are not configured.

## 2026-11-01 - Go Toolchain Alignment in SLSA Release Workflow

**Learning:** The SLSA provenance generation workflow `.github/workflows/go-ossf-slsa3-publish.yml` was configured with default template value `go-version: 1.17`. Since the repository mandates Go 1.25.0 in `go.mod` and leverages modern Go standard libraries and dependencies (`golang.org/x/crypto`), any release build triggered under Go 1.17 will fail immediately.

**Action:** Ensure all release and provenance workflows align with the repository's required Go version (currently `'1.25'`) as dictated in `go.mod` and `.github/workflows/AGENTS.md`.
