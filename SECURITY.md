# Security Policy

## Supported Versions

Security fixes are provided on a best-effort basis.

| Version | Supported |
| --- | --- |
| Latest release | Yes |
| `main` / default branch | Yes |
| Older tags/releases | No |
| Non-default branches/forks | No |

If you need a fix backported to an older release, include the exact version you are running in the report. Backports may not be available depending on risk/effort.

## Reporting a Vulnerability

Please **do not** open a public GitHub issue for security vulnerabilities.

Preferred reporting method:

- Use **GitHub Security Advisories** ("Report a vulnerability"):
  - https://github.com/jimmystewpot/pdns-statsd-proxy/security/advisories/new

If GitHub private reporting is not available for your account, you can report privately by emailing the repository owner via their GitHub profile.

When reporting, please include:

- A clear description of the issue and impact
- Steps to reproduce (PoC), if available
- Affected versions and environment details
- Any relevant logs, configuration snippets (remove secrets), or screenshots
- Suggested remediation, if you have one

If the vulnerability involves the PowerDNS API key or any other credential exposure, please call that out explicitly so we can prioritize mitigation guidance.

## What to Expect

We aim to follow responsible disclosure and respond on a best-effort basis.

- **Initial response**: within **7 days**
- **Status updates**: at least every **14 days** until resolution
- **Fix timeline**: depends on severity and complexity

If the issue is actively exploited or has a high impact, we will prioritize an expedited fix.

## Coordinated Disclosure

Please allow a reasonable amount of time to investigate and release a fix before publicly disclosing the issue.

If you believe a public disclosure deadline is necessary, include the deadline in your report so we can coordinate.

We may publish a GitHub Security Advisory and/or release notes once a fix is available.

## Security Hardening Notes

- Avoid exposing the PowerDNS API endpoint publicly.
- Keep your PowerDNS API key secret and rotate it if you suspect compromise.
- Run this service with the least privilege necessary (dedicated user/service account).
- Keep dependencies up to date and monitor CI findings.
- Consider restricting inbound network access to the proxy itself (firewall/security group) to only trusted monitoring systems.
