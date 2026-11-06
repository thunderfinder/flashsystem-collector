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

## 2026-11-01 - DevSkim Container Build Failure on .NET 7

**Learning:** `microsoft/DevSkim-Action` at commit `914fa647b406c387000300b2f09bb28691be2b6d` (v1.0.14) builds its container from `mcr.microsoft.com/dotnet/sdk:7.0` and attempts `dotnet tool install --tool-path /tools Microsoft.CST.DevSkim.Cli`. The `Microsoft.CST.DevSkim.CLI` package (>= 1.0.100) dropped support for `net7.0`, causing Docker container build failures during CI runs.

**Action:** In `.github/workflows/devskim.yml`, pin `microsoft/DevSkim-Action` to commit `734c6ed89671b1c7a5211ed334c79db261e3f221` (v1) or newer which targets `mcr.microsoft.com/dotnet/sdk:8.0`.

## 2026-11-02 - Race Detection and Job Timeouts in Go CI Workflow

**Learning:** The primary CI workflow `.github/workflows/go.yml` ran `go test -v ./...` without data race detection (`-race`), differing from the documented quality standard in `docs/wiki/Testing-and-Quality.md` and allowing subtle data races in concurrency-critical packages (`internal/cache`, `internal/zabbix`, `internal/ssh`) to pass CI unnoticed. Furthermore, the job omitted an explicit execution timeout, allowing hanging test suites or stuck builds to consume GitHub Actions runner resources indefinitely.

**Action:** Ensure `.github/workflows/go.yml` executes `go test -v -race ./...` and configures `timeout-minutes: 10` on the `build` job.

## 2026-11-03 - Optional Secret Guards in Semgrep Workflow

**Learning:** Optional third-party security workflow `.github/workflows/semgrep.yml` unconditionally ran `returntocorp/semgrep-action` and SARIF upload without verifying if `SEMGREP_APP_TOKEN` was defined. When triggered on pull requests from forks or when optional repository secrets are absent, Semgrep fails the workflow run. In addition, the job lacked an explicit timeout.

**Action:** Ensure `.github/workflows/semgrep.yml` guards action steps with `if: env.SEMGREP_APP_TOKEN != ''` and includes `timeout-minutes: 10`.

## 2026-11-03 - Explicit Job Timeouts in Security Scanning Workflows

**Learning:** Third-party security analysis workflows (`devskim.yml`, `endorlabs.yml`, `sonarqube.yml`, and `veracode.yml`) omitted explicit `timeout-minutes` on their jobs, causing GitHub Actions to fall back to the platform default of 360 minutes (6 hours). When external scanning tools encounter transient network delays, container pull stalls, or API latency, jobs risk hanging and exhausting CI runner capacity.

**Action:** Configure `timeout-minutes: 10` on all third-party security scanning jobs (`lint` in `devskim.yml`, `scan` in `endorlabs.yml`, `Analysis` in `sonarqube.yml`, and `build-and-pipeline-scan` in `veracode.yml`) to enforce deterministic runtime bounds across all CI pipelines.

## 2026-11-04 - Explicit Job Timeouts in Lint and Static Analysis Workflow

**Learning:** The static analysis and workflow verification pipeline `.github/workflows/lint.yml` (`deadcode`, `actionlint`, and `zizmor` jobs) omitted explicit `timeout-minutes`. In the event of external tool package installation delays (e.g. `go install deadcode`, `pip install zizmor`), action download delays, or runner stalls, jobs could execute up to the default 6-hour limit, tying up GitHub Actions concurrency limits.

**Action:** Configure explicit `timeout-minutes: 10` on each job within `.github/workflows/lint.yml` to maintain consistent, deterministic runtime limits across all repository pipelines.

## 2026-11-05 - Explicit Job Timeouts and Go Toolchain Alignment in CodeQL and Dependency Review

**Learning:** The CodeQL analysis workflow (`.github/workflows/codeql.yml`) and Dependency Review workflow (`.github/workflows/dependency-review.yml`) lacked explicit `timeout-minutes`, falling back to GitHub Actions' default 6-hour (360-minute) maximum. In CodeQL, deep recursive AST extraction or external CodeQL pack downloads can stall runner instances. Additionally, CodeQL's `autobuild` build-mode for Go relies on whatever system Go compiler is present in the runner image rather than pinning to the project's required Go version (`go.mod` mandates Go 1.25.0), leading to potential toolchain divergence during Go AST compilation.

**Action:** Configure explicit `timeout-minutes: 30` on the `analyze` job in `.github/workflows/codeql.yml` and `timeout-minutes: 10` on the `dependency-review` job in `.github/workflows/dependency-review.yml`. Furthermore, include an explicit Go setup step (`actions/setup-go` with `go-version: '1.25'`) conditioned on `matrix.language == 'go'` before CodeQL initialization.
