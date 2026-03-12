# Security Policy

## Reporting a Vulnerability

The ngrok Kubernetes Operator team takes security vulnerabilities seriously. We appreciate responsible disclosure and will work with you to investigate and address any issues.

**Please do not report security vulnerabilities through public GitHub issues.**

To report a security vulnerability, please use the [GitHub Security Advisory](https://github.com/ngrok/ngrok-operator/security/advisories/new) feature. This allows you to report the issue privately to the maintainers.

Alternatively, you may email **security@ngrok.com** with:

- A clear description of the vulnerability
- Steps to reproduce the issue
- The potential impact of the vulnerability
- Any suggested mitigations, if known

### What to Expect

- **Acknowledgement**: We will acknowledge receipt of your report within 3 business days.
- **Assessment**: We will investigate and confirm whether the report describes a genuine vulnerability within 10 business days.
- **Resolution**: Once confirmed, we will work on a fix and coordinate a disclosure timeline with you.
- **Credit**: We are happy to credit reporters in release notes and advisories, unless you prefer to remain anonymous.

We ask that you:

- Give us reasonable time to address the issue before any public disclosure.
- Avoid accessing or modifying other users' data during your research.
- Act in good faith to avoid privacy violations, data destruction, or service disruption.

## Supported Versions

Security fixes are applied to the **latest release** of the ngrok Kubernetes Operator. We encourage all users to stay on the most recent version.

| Version | Supported |
|---------|-----------|
| Latest (0.x) | :white_check_mark: |
| Older releases | :x: |

Since the operator is currently pre-1.0.0, only the most recent release branch receives security updates. Users are encouraged to upgrade promptly when new releases are available.

## Scope

This security policy applies to vulnerabilities in the **ngrok Kubernetes Operator** itself, including:

- The operator controller and reconciliation logic
- Helm chart configuration and defaults
- CRD definitions and admission webhooks
- Container images published to the ngrok container registry

### Out of Scope

- Vulnerabilities in the ngrok platform or ngrok agent itself — please report those via [ngrok's security disclosure process](https://ngrok.com/security)
- Vulnerabilities in third-party dependencies — please report those to the relevant upstream projects
- Scanner output that has not been independently verified as exploitable in the context of this operator
- Issues in older, unsupported versions of the operator

## Security Announcements

To stay informed about security updates and advisories:

- Watch this repository for [security advisories](https://github.com/ngrok/ngrok-operator/security/advisories)
- Monitor the [GitHub releases page](https://github.com/ngrok/ngrok-operator/releases) for security-related releases

## Disclosure Policy

When a vulnerability is confirmed, we follow coordinated disclosure:

1. A patch is prepared in a private fork or branch.
2. A GitHub Security Advisory is drafted with CVE assignment requested if applicable.
3. The fix is released and the advisory is published simultaneously.
4. Release notes will reference the advisory.

We aim to disclose vulnerabilities within **90 days** of the initial report, in line with common industry standards.
