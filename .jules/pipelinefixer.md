# PipelineFixer Journal

## 2025-05-18 - Linux Static Build & Go CI Verification Constraints

**Learning:** The IBM FlashSystem Collector target architecture requires static CGO-disabled Linux AMD64 binary compilation (`CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o flashsystem-collector .`). The standard `go.yml` workflow was only running `go build -v ./...` without verifying dependency integrity (`go mod verify`), static analysis (`go vet ./...`), or cross-compilation for Linux AMD64 binaries. Additionally, `golangci-lint` binary builds in local runner environments might mismatch when Go is updated to 1.25+ unless properly aligned.

**Action:** Always ensure `.github/workflows/go.yml` includes Go module caching (`cache: true`), dependency verification (`go mod verify`), static code check (`go vet`), and explicit target Linux AMD64 static binary compilation.

## 2025-05-18 - Environment Variable Scope in Step Conditions

**Learning:** Step-level `if:` conditions in GitHub Actions are evaluated before step-level `env:` blocks are defined. Referencing `env.SONAR_TOKEN` inside a step's `if:` check when `env:` is defined only on that step causes `env.SONAR_TOKEN` to evaluate as empty string. Defining `env:` at the job level allows step `if:` conditions to evaluate environment variables correctly.

**Action:** Define secrets and environment variables at the job level (`jobs.<job_id>.env`) when referencing them in step-level conditional expressions (`if: env.VAR_NAME != ''`).
