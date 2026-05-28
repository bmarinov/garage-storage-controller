# Security

## Reporting vulnerabilities

Please report vulnerabilities by opening a [GitHub issue](https://github.com/bmarinov/garage-storage-controller/issues).

## Dependency vulnerability policy

Vulnerabilities are tracked with `govulncheck` via `scripts/govulncheck.sh`. The wrapper filters out excluded findings with documented tracking issues.

### Known exclusions

The following vulnerabilities are in transitive dependencies with no upstream fix available and are excluded from CI checks:

| ID | CVE | Package | Reason |
|----|-----|---------|--------|
| GO-2026-4887 | CVE-2026-34040 | `github.com/docker/docker` | Test dependency; no fix released. Tracked: [#72](https://github.com/bmarinov/garage-storage-controller/issues/72) |
| GO-2026-4883 | CVE-2026-33997 | `github.com/docker/docker` | Test dependency; no fix released. Tracked: [#72](https://github.com/bmarinov/garage-storage-controller/issues/72) |

`docker/docker` is a transitive dependency for `testcontainers-go`. Only used in integration tests.

### Go standard library

Stdlib vulnerabilities are fixed by upgrading the Go toolchain. This module requires **go 1.26** as a minimum language version. Build with **go 1.26.3 or later** to include fixes for:

| ID | Package | Description |
|----|---------|-------------|
| GO-2026-4918 | `net/http` | HTTP/2 SETTINGS infinite loop (CVE-2026-33814) |
| GO-2026-4971 | `net` | (see Go 1.26.3 release notes) |
| GO-2026-4977 | `net/mail` | Quadratic string concatenation in `consumePhrase` |
| GO-2026-4986 | `net/mail` | Quadratic string concatenation in `consumeComment` |
| GO-2026-4980 | `html/template` | Escaper bypass leading to XSS |
| GO-2026-4982 | `html/template` | Meta content URL escaping bypass leading to XSS |

These affect the Go toolchain itself, not this module's dependencies. 
