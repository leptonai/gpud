# Security Policy

## Supported Versions

The following versions of gpud are currently supported with security updates:

| Version | Supported          |
| ------- | ------------------ |
| latest  | :white_check_mark: |
| < 1.0   | :x:                |

## Reporting a Vulnerability

We take security vulnerabilities seriously. If you discover a security issue in gpud, please report it responsibly.

### How to Report

**Please do NOT report security vulnerabilities through public GitHub issues.**

Instead, please report security vulnerabilities by emailing:

- **Email:** security@lepton.ai

Please include the following information in your report:

1. **Type of issue** (e.g., buffer overflow, SQL injection, cross-site scripting, etc.)
2. **Full paths of source file(s)** related to the manifestation of the issue
3. **The location of the affected source code** (tag/branch/commit or direct URL)
4. **Any special configuration** required to reproduce the issue
5. **Step-by-step instructions** to reproduce the issue
6. **Proof-of-concept or exploit code** (if possible)
7. **Impact of the issue**, including how an attacker might exploit it

### What to Expect

- **Acknowledgment:** We will acknowledge receipt of your vulnerability report within 48 hours.
- **Communication:** We will keep you informed of the progress toward a fix and full announcement.
- **Credit:** We will credit you in the security advisory if you wish (please let us know your preference).
- **Timeline:** We aim to release a fix within 90 days of the initial report, depending on the severity and complexity.

### Preferred Languages

We prefer all communications to be in English.

## Security Best Practices

When deploying gpud in production environments, we recommend:

1. **Run with least privileges:** Use non-root users where possible
2. **Network isolation:** Restrict network access to the gpud API endpoints
3. **Keep updated:** Regularly update to the latest version for security patches
4. **Audit logs:** Enable and monitor gpud logs for suspicious activity
5. **Container security:** When running in Docker, follow container security best practices

## Security Advisories

Security advisories will be published through:
- GitHub Security Advisories on this repository
- Release notes for versions containing security fixes

## Acknowledgments

We would like to thank the following security researchers for their responsible disclosure:

*None yet - be the first!*
