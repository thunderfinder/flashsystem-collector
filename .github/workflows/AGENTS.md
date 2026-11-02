# AGENTS.md — .github/workflows Component Instructions

This directory contains GitHub Actions workflows defining the Continuous Integration (CI), code quality, and security scanning pipelines for `flashsystem-collector`.

---

## Technical Responsibilities

- **Build & Test Pipelines**: `go.yml` enforces compilation, module cache, `go vet`, and test suites.
- **Linters**: `lint.yml` executes `deadcode`, `actionlint` (workflow linter), and `zizmor` (GitHub Actions security linter).
- **Security Scanners**: Workflows for CodeQL, DevSkim, Semgrep, Dependency Review, SonarQube, Endor Labs, and Veracode.
- **Supply-Chain Security**: `go-ossf-slsa3-publish.yml` generates Level 3 SLSA provenance for release binaries.

---

## Engineering Rules & Toolchain Constraints (PipelineFixer Learnings)

1. **Go Version Consistency**:
   - `go-version` in `go.yml` and `go-ossf-slsa3-publish.yml` must explicitly match Go version specified in `go.mod` (`1.25` / `1.25.0`).
2. **zizmor Workflow Linter Configuration**:
   - Uses `.github/zizmor.yml` (`unpinned-uses: ref-pin` and `artipacked: false`) to prevent CI failures from tag-pinned actions and read-only checkout tokens.
3. **DevSkim Action Pinning**:
   - `microsoft/DevSkim-Action` must pin to commit `734c6ed89671b1c7a5211ed334c79db261e3f221` (v1) or newer supporting .NET 8 SDK, as older releases pinned to .NET 7 fail container build.
4. **Conditional Secret Handling**:
   - Optional third-party security workflows (SonarQube, Endor Labs, Veracode) must include step- or job-level secret conditionals (`if: env.SONAR_TOKEN != ''`) to prevent workflow failure when secrets are not configured in forks or public PRs.
5. **No Secret Leaks**:
   - Never print GitHub secrets, tokens, or private keys to workflow job logs.
6. **No Weakening Checks**:
   - Never disable or weaken CI checks to bypass linter warnings or security scanner findings.

---

## Validation Guidelines

- Review any modified workflow using `git diff` and check YAML syntax formatting.
- Lint workflows locally using `actionlint` if installed.
- Confirm required secrets and environment variables are documented clearly.
