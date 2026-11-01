# Contributing 🤝

Contributions to `thunderfinder/flashsystem-collector` are welcome! This document establishes expectations, testing requirements, and review processes.

---

## 1. Development Principles

1. **Safety First**: The collector interacts with enterprise storage controllers. It must remain strictly read-only (`svcinfo` only; never `svctask`).
2. **Deterministic Output**: Output to stdout must always be valid JSON. Never output unstructured text, panics, or log lines to stdout.
3. **Fail-Safe Caching**: Any new collector domain must respect TTL boundaries and implement clean stale-data fallback behavior.
4. **Zero Regressions**: All tests must pass with race detection enabled (`go test -race ./...`).

---

## 2. Pull Request Workflow

1. **Fork and Branch**: Create a descriptive feature branch (e.g., `feature/add-lshost-collector` or `fix/parser-multiline`).
2. **Code Standards**: Run `gofmt` and ensure compliance with project naming conventions.
3. **Add Tests**: Provide unit tests under `internal/<pkg>/` verifying both successful parsing and malformed input edge cases.
4. **Run Verification**:
   ```bash
   go vet ./...
   go test -race ./...
   CGO_ENABLED=0 go build -trimpath .
   ```
5. **Update Documentation**: Update the corresponding Wiki pages in `docs/wiki/` (and update `_Sidebar.md` if new pages are added).
6. **Submit PR**: Open a pull request against `main` with a clear description of the problem solved.

---

## 3. Reporting Security Vulnerabilities

If you discover a security vulnerability (such as a flaw in SSH key handling or path traversal in cache creation), please do not open a public issue. Follow standard responsible disclosure practices by emailing the maintainers directly or utilizing GitHub Security Advisories.
