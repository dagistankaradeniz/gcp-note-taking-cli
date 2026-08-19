# Security Policy

Quillink treats security and privacy as the core product, not an add-on. If
you've found a vulnerability, we want to hear about it before anyone else
does.

## Reporting a vulnerability

Email **security@quillink.app** with:

- A description of the issue and its potential impact.
- Steps to reproduce (a proof-of-concept, request/response samples, or a
  short script — whatever gets us there fastest).
- The component affected (backend API, web app, iOS app, Android app, CLI,
  or MCP server) and, if relevant, the account/environment you tested
  against.

**Response times** (best-effort, not contractual): acknowledgment within 3
business days, an initial assessment within 10 business days, and we'll
keep you updated on remediation progress until it's resolved.

## Scope

**In scope:**
- The backend API and Firestore/Storage security rules
  (`gcp-note-taking-backend`).
- The web app (`gcp-note-taking-frontend`).
- The iOS and Android apps.
- The CLI and MCP server.
- Any subdomain of `quillink.app`.

**Out of scope:**
- Denial-of-service or volumetric attacks — don't run load/stress tests
  against production.
- Social engineering, phishing, or physical attacks against Quillink staff
  or infrastructure providers.
- Automated vulnerability scanners' raw output without a demonstrated,
  exploitable impact (e.g. a missing security header alone, with no proof
  of exploitability, isn't actionable on its own — tell us what it lets you
  actually do).
- Reports concerning third-party services we integrate with (Google
  Cloud/Firebase, Gemini, Resend) — report those directly to the vendor.
- Vulnerabilities requiring physical access to a user's unlocked device.

## Safe harbor

If you make a good-faith effort to comply with this policy while
researching and reporting a vulnerability, we will not pursue legal action
against you for that research. Please:

- Only interact with accounts you own or have explicit permission to test.
- Avoid privacy violations, data destruction, and service disruption.
- Give us a reasonable window to fix an issue before any public disclosure
  (90 days is a reasonable default; we'll work with you if more time is
  genuinely needed).

## No paid bounty program (yet)

We don't currently run a paid bug bounty program — this is a young,
independently-run product, not a funded security budget yet. We will
credit you (with your permission) in release notes for any vulnerability
you responsibly disclose, and we take every report seriously regardless of
whether there's a monetary reward attached.
