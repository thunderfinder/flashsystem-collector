# PipelineFixer Journal

## 2025-05-18 - Linux Static Build & Go CI Verification Constraints

**Learning:** The IBM FlashSystem Collector target architecture requires static CGO-disabled Linux AMD64 binary compilation (`CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o flashsystem-collector .`). The standard `go.yml` workflow was only running `go build -v ./...` without verifying dependency integrity (`go mod verify`), static analysis (`go vet ./...`), or cross-compilation for Linux AMD64 binaries. Additionally, `golangci-lint` binary builds in local runner environments might mismatch when Go is updated to 1.25+ unless properly aligned.

**Action:** Always ensure `.github/workflows/go.yml` includes Go module caching (`cache: true`), dependency verification (`go mod verify`), static code check (`go vet`), and explicit target Linux AMD64 static binary compilation.

## 2025-05-18 - Optional Secret Guards & Security Integration Placeholders

**Learning:** Optional third-party integration workflows like `sonarqube.yml`, `endorlabs.yml`, `veracode.yml`, and `lint.yml` fail when required secrets or configuration variables (such as `SONAR_TOKEN`, `ENDOR_NAMESPACE`, `VERACODE_API_ID`) are missing or left as default placeholders (e.g. `namespace: "example"`). Using prebuilt `golangci-lint` binaries built on Go 1.24 also fails when target `go.mod` specifies Go 1.25+.

**Action:** Guard third-party security workflows with `if: env.<SECRET_VAR> != ''` or `if: vars.<CONFIG_VAR> != ''` at job or step levels so CI jobs pass when optional external services are not configured.

## 2026-11-01 - DevSkim Container Build Failure with .NET 7.0

**Learning:** `microsoft/DevSkim-Action@914fa647b406c387000300b2f09bb28691be2b6d` (v1.0.14) builds its container using base image `mcr.microsoft.com/dotnet/sdk:7.0`. When `dotnet tool install --tool-path /tools Microsoft.CST.DevSkim.Cli` runs, NuGet resolves `Microsoft.CST.DevSkim.CLI >= 1.0.100`, which no longer supports `net7.0` (it only supports `net8.0`, `net9.0`, and `net10.0`), failing the Docker build with `NU1202`. Pinning to commit `734c6ed89671b1c7a5211ed334c79db261e3f221` (v1.0.15) updates the Dockerfile to use `.NET 8.0 SDK`, resolving the installation incompatibility.

**Action:** Always pin `microsoft/DevSkim-Action` to commit `734c6ed89671b1c7a5211ed334c79db261e3f221` (v1.0.15) or newer that builds against .NET 8+.
