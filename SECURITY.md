# Security Policy

## Reporting a vulnerability

If you discover a security vulnerability in the Bandwidth CLI, **do not open a public issue.** Instead:

1. **Preferred:** Report through our [Bug Bounty Program](https://www.bandwidth.com/security/report-a-vulnerability/), which is managed through Bugcrowd for initial triage.
2. **Alternatively:** Email [security@bandwidth.com](mailto:security@bandwidth.com) with details.

We'll acknowledge your report within 5 business days and aim to provide a fix or mitigation plan within 30 days, depending on severity.

## What counts as a vulnerability

- Authentication bypass or credential leakage in the CLI itself
- Command injection or code execution through CLI inputs
- Insecure storage of credentials or tokens
- Privilege escalation through the CLI

## What doesn't count

**A dependency having a CVE does not automatically mean the CLI is vulnerable.** We use `govulncheck` in CI, which checks whether vulnerable code paths are actually reachable from our code — not just whether a dependency version appears in a database.

If you're reporting a dependency CVE, please include:

- The specific call chain from `band` code into the vulnerable function, or
- A proof of concept showing the vulnerability is exploitable through the CLI

Reports that only list a dependency version and a CVE number without demonstrating reachability will need additional context before we can act on them.

## Supported versions

We support the latest released version. Security fixes are not backported to older releases. Upgrade to the latest version to get fixes.
