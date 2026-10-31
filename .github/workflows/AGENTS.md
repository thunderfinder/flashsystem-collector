# AGENTS.md — .github/workflows Component Instructions

This directory contains GitHub Actions CI/CD workflows for building, testing, linting, and security scanning `flashsystem-collector`.

---

## Workflows Overview

The repository defines 10 GitHub Actions workflows:

### Build, Test & Lint
- `go.yml`: Compiles Go code across supported setups, runs `go test ./...`, and validates static Linux AMD64 build.
- `lint.yml`: Code quality checks, running `deadcode` from `golang.org/x/tools/cmd/deadcode`.

### Security, Supply Chain & Static Analysis
- `codeql.yml`: GitHub CodeQL Advanced static analysis for Go code.
- `dependency-review.yml`: Scans dependency manifest files that change in pull requests against known vulnerabilities.
- `devskim.yml`: Microsoft DevSkim security linter.
- `endorlabs.yml`: Endor Labs dependency and supply chain security scanner.
- `go-ossf-slsa3-publish.yml`: SLSA Build Level 3 provenance generator for Go release binaries using `slsa-github-generator`.
- `semgrep.yml`: Semgrep SAST scanning with default rules.
- `sonarqube.yml`: SonarQube code quality and vulnerability scan (triggered on push/PR).
- `veracode.yml`: Veracode static analysis pipeline scan converting results to SARIF.

---

## Engineering & Maintenance Rules

1. **Go Version Alignment**:
   - The Go toolchain version across workflow steps (e.g., `actions/setup-go@v5`) must align with `go.mod` (Go `1.25.0`).
2. **Least Privilege Permissions**:
   - Workflows must declare explicit top-level or job-level `permissions:` (e.g., `contents: read`, `security-events: write`, `id-token: write`).
3. **Third-Party Secret & Variable Guards**:
   - Workflows requiring external third-party services (such as `sonarqube.yml`, `endorlabs.yml`, and `veracode.yml`) must include appropriate job-level or step-level conditions (such as checking if secrets or environment variables are non-empty) to prevent pipeline failures in forks or open-source environments where secrets are not configured.
4. **Supply Chain Integrity**:
   - Pin external GitHub Actions with verified version tags (e.g., `@v4`, `@v5`) or immutable commit SHAs.
   - Maintain reproducible builds for release pipelines using SLSA standards.
5. **No Functional Application Changes**:
   - Do not weaken CI checks to bypass linter warnings or security scanner findings.

---

## Validation Guidelines

- Review any modified workflow using `git diff` and check YAML syntax formatting.
- Confirm required secrets and environment variables are documented clearly.
