# Security policy

iMail is a self-hosted mail server: it speaks SMTP, submission, SMTPS, IMAP,
POP3 and POP3S, serves a web mailbox and an administration console from one Go
binary, and keeps configuration, database, DKIM key and TLS material in a single
`config/` volume. That combination is worth attacking, and reports are welcome.

## Supported versions

Only the current `main` branch is supported. It is published as
`ghcr.io/noelbennettz/imail:latest`, also tagged `main` and `sha-<commit>`.
Older images and forks get fixes only if the report also reproduces on the
current `main`.

## Reporting a vulnerability

Use GitHub's private reporting: the **Security** tab of
<https://github.com/noelbennettz/imail>, then **Report a vulnerability**. The
report stays private until an advisory is published.

If that form is not available, open a normal issue whose title is exactly
`Security contact request` and whose body says nothing about the technical
details, the affected endpoints, the reproduction steps or the impact. A
maintainer will open a private channel in the issue and continue there.

Please do not open a public issue with the details, and do not test against
deployments you do not own.

## What to expect

There is no response-time guarantee: this is a volunteer-maintained project
without a support contract or a security team on call. Reports are read and
answered in the order they arrive. A report that includes the affected version
or commit, a reproduction on a fresh deployment, and the impact you observed is
much easier to act on, and therefore usually fixed sooner.

## In scope

- The SMTP, submission, SMTPS, IMAP, POP3 and POP3S listeners, and the
  authentication they share with the web interface.
- The web mailbox and the administration console, including session handling,
  access control, attachment handling and the stored-mailbox views.
- The setup wizard: unauthenticated exposure of its endpoints, the one-time
  setup token, and what it lets an unauthenticated visitor reach.
- The TLS and ACME flows: certificate issuance, the HTTP-01 challenge endpoint,
  certificate storage and the TLS listener configuration.
- DKIM keys and every other secret that lives in `config/`, plus the file modes
  and backup handling that protect them.

## Out of scope

- Mail reputation, spam scoring and deliverability. A message that is accepted
  but lands in a spam folder is not a vulnerability.
- Providers and networks that block port 25, or that refuse to lift the block.
- DNS records that you configured incorrectly, including missing MX, SPF, DKIM
  or DMARC entries, and propagation delays.
- Vulnerabilities in upstream dependencies. Report those to the upstream
  project; iMail will pick up the fixed release.

## Scope of this document

This is the security policy of the project, not a warranty. It describes how
reports are handled and what the deployment itself asks of you; the
[security notes in DOCS.md](DOCS.md#security-notes) describe the behaviour you
can verify in the code.
