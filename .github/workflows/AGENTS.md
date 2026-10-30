# AGENTS.md — .github/workflows Component Instructions

This directory contains GitHub Actions CI/CD workflows for building, testing, linting, and security scanning the project.

---

## Workflows Overview

- `go.yml` / `Pipery Go CI.yml` / `Setup Go environment.yml`: Go compilation and unit test execution workflows.
- `cache.yml`: Dependency caching workflows.
- Security & Static Analysis Workflows:
  - `codeql.yml`: GitHub CodeQL static analysis.
  - `crda.yml`: Red Hat CodeReady Dependency Analytics.
  - `dependency-review.yml`: Dependency review action for PRs.
  - `devskim.yml`: DevSkim security linter.
  - `endorlabs.yml`: Endor Labs supply chain security.
  - `go-ossf-slsa3-publish.yml`: SLSA provenance generation.
  - `golang-govulncheck-action.yml`: Go vulnerability scanner.
  - `semgrep.yml`: Semgrep SAST scanning.
  - `sonarqube.yml`: SonarQube code quality analysis.
  - `veracode.yml`: Veracode static analysis.

---

## Engineering & Maintenance Rules

1. **Go Version Alignment**:
   - Ensure the `go-version` configured across GitHub Actions steps aligns with `go.mod` requirement (`1.25.0`).
2. **Least Privilege Permissions**:
   - Maintain explicit `permissions:` blocks (e.g., `contents: read`, `security-events: write`) for all workflow jobs.
3. **Secret Security**:
   - Never hardcode or print secrets, tokens, or credentials in CI steps. Use GitHub Secrets (`${{ secrets.* }}`).
4. **Supply Chain Security**:
   - Pin third-party GitHub Actions to specific release versions or full commit SHA hashes.

---

## Testing & Validation Guidelines

- Test workflow modifications locally using `act` (if available) or via feature branch pull requests.
- Ensure all CI workflows pass before merging pull requests.
