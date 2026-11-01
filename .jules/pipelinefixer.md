# PipelineFixer Journal

## 2025-05-18 - Linux Static Build & Go CI Verification Constraints

**Learning:** The IBM FlashSystem Collector target architecture requires static CGO-disabled Linux AMD64 binary compilation (`CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o flashsystem-collector .`). The standard `go.yml` workflow was only running `go build -v ./...` without verifying dependency integrity (`go mod verify`), static analysis (`go vet ./...`), or cross-compilation for Linux AMD64 binaries. Additionally, `golangci-lint` binary builds in local runner environments might mismatch when Go is updated to 1.25+ unless properly aligned.

**Action:** Always ensure `.github/workflows/go.yml` includes Go module caching (`cache: true`), dependency verification (`go mod verify`), static code check (`go vet`), and explicit target Linux AMD64 static binary compilation.

## 2025-05-18 - Optional Secret Guards & Security Integration Placeholders

**Learning:** Optional third-party integration workflows like `sonarqube.yml`, `endorlabs.yml`, `veracode.yml`, and `lint.yml` fail when required secrets or configuration variables (such as `SONAR_TOKEN`, `ENDOR_NAMESPACE`, `VERACODE_API_ID`) are missing or left as default placeholders (e.g. `namespace: "example"`). Using prebuilt `golangci-lint` binaries built on Go 1.24 also fails when target `go.mod` specifies Go 1.25+.

**Action:** Guard third-party security workflows with `if: env.<SECRET_VAR> != ''` or `if: vars.<CONFIG_VAR> != ''` at job or step levels so CI jobs pass when optional external services are not configured.

## 2026-11-01 - DevSkim-Action .NET Runtime Version Pinning Failure

**Learning:** `microsoft/DevSkim-Action@914fa647b406c387000300b2f09bb28691be2b6d` (v1.0.14) builds its container using `dotnet tool install --tool-path /tools Microsoft.CST.DevSkim.Cli` from an unpinned package source on a .NET 7.0 SDK image. Newer releases of `Microsoft.CST.DevSkim.Cli` (>= 1.0.100) dropped support for `net7.0`, causing Docker container build failures during CI execution (`Package Microsoft.CST.DevSkim.CLI 1.0.100 is not compatible with net7.0`). The official DevSkim-Action `v1` release was updated to commit `734c6ed89671b1c7a5211ed334c79db261e3f221` which upgrades the Dockerfile to .NET 8.0 SDK and pins the CLI to `--version 1.0.90`.

**Action:** Pin `microsoft/DevSkim-Action` to commit `734c6ed89671b1c7a5211ed334c79db261e3f221` (# v1) to prevent Docker container build failures from unpinned NuGet dependency drift.
