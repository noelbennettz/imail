# iMail documentation

> Applies to the current `main`, published as `ghcr.io/noelbennettz/imail:latest`.
> Older releases keep their documentation on the project site under
> `versions/<tag>/`.

iMail is a self-hosted mail server for your own domains: one Go binary with an
embedded Vue 3 client. [README.md](README.md) carries the requirements and the
shortest install path; this file is the reference for everything else.

![The iMail sign-in screen](https://noelbennettz.github.io/imail/ui-login.png)

## Contents

- [Features](#features)
- [Installation](#installation)
- [DNS records](#dns-records)
- [Ports and TLS](#ports-and-tls)
- [Using the mailbox](#using-the-mailbox)
- [Administration](#administration)
- [Certificates](#certificates)
- [Configuration reference](#configuration-reference)
- [Docker and operations](#docker-and-operations)
- [Backups and upgrades](#backups-and-upgrades)
- [Security notes](#security-notes)
- [Troubleshooting](#troubleshooting)
- [Development](#development)

## Features

- **SMTP** on 25 (STARTTLS), **submission** on 587 (STARTTLS), **SMTPS** on
  465, **IMAPS** on 993, **POP3** on 110 (STLS required for authentication) and
  **POP3S** on 995.
- **Dual-stack listeners**: `listenMode` can be `both`, `ipv4` or `ipv6`.
  Outbound delivery follows `egressMode` (`both`, `ipv4` or `ipv6`); a
  configured family the host does not have is detected at startup and the other
  one is used instead.
- **TLS 1.2 and TLS 1.3 only**, with strict SNI validation by default. A TLS
  connection with no SNI or an unknown hostname is rejected during the
  handshake. `serverHostnames` adds extra names when needed.
- **Multiple domains and mailboxes**, with a primary domain, per-mailbox
  quotas, per-domain user administration and per-domain TLS.
- **ACME certificates** with HTTP-01, manual DNS-01 TXT records, or automatic
  DNS-01 through a Cloudflare API token. Certificates can also be uploaded.
- **DKIM signing**, SPF checking, DMARC/PTR guidance and a configurable spam
  filter level.
- **Material 3 web UI**: light/dark/system themes, responsive mobile layout,
  search, conversation grouping, message view, reply, reply-all, forward and
  attachment download.
- **Conversation view** loads five messages at a time. Scroll to the bottom or
  press “Load more” to continue. Each message has its own menu for viewing the
  original source or deleting that single message.
- **Seven UI languages** — English, Arabic, Spanish, French, Russian and
  Simplified Chinese (the six official languages of the United Nations) plus
  Traditional Chinese. All UI assets, including the Inter font, are served
  locally from the binary.
- **SQLite by default**, with MySQL and PostgreSQL schema support for larger
  deployments.
- **UTC in storage, local time in the browser**: message timestamps are stored
  in UTC and returned as RFC 3339; the web client renders them in the device’s
  timezone.


## Installation

The requirements — Docker with Compose (or Go 1.24+), a public IPv4 and/or IPv6
address, a domain with DNS control, the inbound and outbound ports, and port 80
for ACME — are listed in [README.md](README.md#requirements). This section is
the long form of the two supported deployments.

### Build locally with Docker Compose

```bash
git clone https://github.com/noelbennettz/imail.git
cd imail
mkdir -p config
docker compose up -d --build
docker compose logs -f imail
```

`docker-compose.yml` builds the image from the working tree, publishes the eight
ports and mounts `./config` at `/work/config`. The `config/` directory is
created by the container on first start; keep it persistent and back it up.

### Run the published image with `docker run`

The GitHub Actions workflow builds `linux/amd64` and `linux/arm64` images and
publishes them to GitHub Container Registry. On a host with Docker:

```bash
mkdir -p /srv/imail/config
docker pull ghcr.io/noelbennettz/imail:latest
docker run -d \
  --name imail \
  --restart unless-stopped \
  -p 25:25 \
  -p 80:80 \
  -p 110:110 \
  -p 443:443 \
  -p 465:465 \
  -p 587:587 \
  -p 993:993 \
  -p 995:995 \
  -v /srv/imail/config:/work/config \
  -e TZ=UTC \
  ghcr.io/noelbennettz/imail:latest
```

`/srv/imail/config` is the persistent volume. It must be writable by the
container and should be backed up: it contains the configuration, database,
DKIM key and TLS certificates.

Then open `http://<server-ip>/` and complete the setup wizard. Its first screen
asks for the setup token printed in `docker compose logs imail`. The same ports
are used by the Compose example; on rootless Docker publish high host ports
instead (`-p 8080:80 -p 18443:443`, and so on).

### Run with Docker Compose from GHCR

`docker-compose.ghcr.yml` pulls the published image instead of building
locally. It uses the same ports and `./config` volume:

```bash
mkdir -p config
docker compose -f docker-compose.ghcr.yml pull
docker compose -f docker-compose.ghcr.yml up -d
docker compose -f docker-compose.ghcr.yml logs -f imail
```

Update this deployment with:

```bash
docker compose -f docker-compose.ghcr.yml pull
docker compose -f docker-compose.ghcr.yml up -d
```

The `config/` directory is created by the container on first start; keep it
persistent and back it up.

### Setup wizard

Open `http://<server-ip>/` and complete the wizard:

1. **Setup token** — the server prints a one-time token at startup, in a banner
   containing the line `Read it again with: docker compose logs imail`. Copy that
   24-character token (letters and digits) into the first screen. The token is
   generated only while the deployment is uninitialised, exists in memory only,
   is never written to `config/`, and is discarded once setup succeeds. Every
   wizard request carries it as the `X-Setup-Token` header. Missing or wrong
   tokens get `403`; after five failures from one address, further attempts get
   `429` for fifteen minutes, and more than twenty failures in fifteen minutes
   pause verification for everybody until the window drains. Only the static
   client, `/api/ping` and a minimal status endpoint answer without a token.
2. **Database** — SQLite is the default and works well for a small server.
   Choose MySQL or PostgreSQL if you already operate one.
3. **Administrator** — choose an administrator account and a password of at
   least 8 characters. Administrator accounts also use the mailbox login rules
   described below.
4. **Domains** — set the primary mail domain, the browser-facing web domain
   (for example `mail.example.com`) and any additional domains.
5. **DNS** — copy the generated records into your DNS provider. The wizard
   shows A/AAAA, MX, SPF, DKIM, DMARC and PTR guidance.
6. **TLS** — choose HTTP-01, DNS-01 (manual or Cloudflare) or an uploaded
   certificate.

After the wizard finishes, iMail restarts its listeners and the mailbox is
available at the web domain. The setup wizard is only available before the
first successful initialization; complete it promptly and do not expose a
fresh, unconfigured instance to an untrusted network. The wizard itself is
served over plain HTTP — it runs before a certificate exists — so the
administrator password set in step 3 crosses the network in clear text; run the
first setup from the machine's console, an SSH tunnel or a VPN, not over the
public internet. The setup token closes the race with a stranger who reaches
the port first; it does not encrypt anything in transit.

### First login

- With **one configured domain**, you may sign in with the bare account name
  or the full address (`alice` or `alice@example.com`).
- With **two or more configured domains**, sign in with the full address
  (`alice@example.com`). A bare name is rejected because the mailbox domain
  would be ambiguous.
- The same rule is used by the web UI, SMTP AUTH, IMAP and POP3.
- Accounts are globally unique: `account` is a unique column in the user table
  and the domain is a separate field. If you need the same local part in two
  domains, use distinct account names (for example `alice` and `alice-sales`);
  `alice@one.example` and `alice@two.example` cannot coexist.

## DNS records

Let `example.com` be the primary domain and `mail.example.com` the web domain.
The setup wizard generates the exact values for your server; the table below is
the shape of the records. Repeat the mail records for every domain iMail should
receive mail for.

| Host | Type | Value | Notes |
|---|---|---|---|
| `@` | A / AAAA | public server address | Apex for mail delivery |
| `mail` | A / AAAA | public server address | Web UI; use your `webDomain` |
| `smtp` | A / AAAA | public server address | SMTP clients and MX target |
| `imap` | A / AAAA | public server address | IMAP clients |
| `pop` | A / AAAA | public server address | POP3 clients |
| `@` | MX | `smtp.example.com.` | Preference 10; inbound mail |
| `@` | TXT | `v=spf1 a mx ~all` | SPF |
| `default._domainkey` | TXT | generated DKIM public key | Use the value from the wizard, not this text |
| `_dmarc` | TXT | `v=DMARC1; p=quarantine; rua=mailto:postmaster@example.com` | DMARC |
| reverse DNS / PTR | PTR | `smtp.example.com` | Set at the VPS or IP provider; cannot be set in normal domain DNS |

The setup wizard also shows the records for each additional domain. For
multi-domain setups, each domain needs its own MX, SPF, DKIM and DMARC records
and the mail hostname must resolve to this server. Keep the DKIM private key
and the generated `config/` directory private; only the public key belongs in
DNS.

Propagation can take longer than the wizard. Validate with:

```bash
dig +short MX example.com
dig +short TXT default._domainkey.example.com
dig +short TXT _dmarc.example.com
dig +short A smtp.example.com
dig -x <server-ip>
```

## Ports and TLS

### Listener map

| Port | Protocol | Notes |
|---:|---|---|
| 25 | SMTP | Inbound mail from other servers; STARTTLS is offered and SASL AUTH is accepted only over TLS |
| 80 | HTTP | Web UI in HTTP mode and ACME HTTP-01 challenges |
| 110 | POP3 | Generic plaintext greeting; **STLS is required before USER/PASS** |
| 443 | HTTPS | Web UI when `httpsEnabled` is not `2` |
| 465 | SMTPS | Implicit TLS |
| 587 | Submission | STARTTLS |
| 993 | IMAPS | Implicit TLS; IMAP is not exposed without TLS |
| 995 | POP3S | Implicit TLS |

The Docker image exposes all eight ports. `docker-compose.yml` publishes the
same numbers on the host. On a rootless Docker host, publish high host ports and
map them to the container ports (for example `8080:80` and `18443:443`) because
rootless Docker cannot bind ports below 1024.

Port 25 is not optional for inbound mail. An MX record carries no port, so a
remote server always connects to port 25 on the host it resolves; a listener on
another port is usable by clients you configure yourself (for example your own
users submitting on 587), not by the internet. If your provider blocks inbound
25, put a relay or a host with 25 open in front and point the MX record there.

### Address families

`listenMode` controls which sockets are bound: `both` (Go’s dual-stack
wildcard), `ipv4` (`0.0.0.0`) or `ipv6` (`[::]`, IPv6-only). `egressMode`
controls the family used for outbound SMTP delivery to MX hosts. In `both`
mode the outbound connection uses Happy Eyeballs, so a destination that only
publishes an AAAA record is reached over IPv6 and an A-only destination over
IPv4.

IPv6-only and IPv4-only hosts are supported. If `config.json` requests a family
the host cannot bind, the server falls back to the available family at startup
and logs a warning; the listeners, the loopback readiness probe and the A/AAAA
DNS guidance then use the effective family, while the container healthcheck
probes both loopbacks. A host that cannot reach one family at all naturally
cannot deliver to a destination that only exists on that family; the failure is
reported as a normal temporary delivery error and the next MX is tried.

### `httpsEnabled`

`httpsEnabled` controls the web listener:

- `0` / `1` — HTTPS is enabled: the HTTPS listener starts on `httpsPort`
  (default 443), and the HTTP listener on `httpPort` (default 80) answers only
  `/api/ping` and `/.well-known/` for ACME HTTP-01, redirecting every other
  request with `301` to `https://<webDomain><path>`.
- `2` — HTTP-only mode: the HTTPS listener does not start and the HTTP listener
  serves the whole application. Use this behind a TLS reverse proxy or for local
  testing. The session cookie is not marked `Secure` in this mode; terminate
  TLS at a trusted proxy when using HTTP-only mode in production.

Mail listeners always use the configured certificates. The web domain and all
mail hostnames must be present in the certificate’s SAN list. If you use a
reverse proxy for the web UI, either preserve the original hostname and SNI or
add the public hostname to `serverHostnames`; do not disable SNI validation
unless you fully understand the exposure.


## Delivery and retries

iMail keeps a durable outbound queue for mail addressed to other domains, for
both submission paths: an SMTP client and the webmail compose window. The
message is accepted (`250`, or a successful API response) once the message and
its queue rows are committed, and a background worker delivers it from then on.
Messages to mailboxes this server hosts are still delivered inline.

One queue row is written per remote **domain**, not per message. A domain that
accepted the message is never contacted again while another domain is still
being retried.

- **Durability**: the body is written to the database before `DATA` is
  acknowledged. The message row and the queue row commit in one transaction, so
  either both exist or the client gets `451` and retries. A crash between the
  commit and the delivery therefore loses nothing.
- **Order**: the recipient domain's MX hosts are tried in priority order. Each
  MX is tried on port 25 (STARTTLS), then 587 (STARTTLS), then 465 (implicit
  TLS). A 5xx answer from a remote server stops the attempt for that domain;
  anything else moves on to the next MX (RFC 5321 §5.1).
- **Retries**: a temporary failure (network error, 4xx reply) is retried after
  1 minute, 5 minutes, 15 minutes, 1 hour, 6 hours and then every 24 hours. An
  entry is given up when the next attempt would fall outside 72 hours from
  acceptance. A permanent 5xx reply fails the entry immediately.
- **Status**: the sent-folder row stays *delivery pending* while any queue row is
  waiting, becomes *sent* when every remote domain accepted the message, and
  *failed* (with the last error) when a domain finally gave up. Every attempt is
  logged with the queue id, the message id and the recipients.
- **Outbox panel**: the Security page under Administration shows the number of
  waiting and finally failed queue rows — one per remote domain — and lets an
  administrator inspect them. The list is paged and shows the recipients, the
  attempt count, the last error and the last update. A **failed** row can be
  retried (it goes back to the queue at once and its 72-hour retry window
  restarts) or deleted. A delivered row can be deleted as well. Pending and
  in-flight rows cannot be deleted: the worker still owns them, and deleting
  one would silently drop mail. Every action is admin-only (the API endpoints
  are `/api/admin/outbox/list`, `/retry` and `/delete`; other users get the
  ordinary “no access” response).
- **Restart**: pending entries are stored in the database and the worker claims
  them again at startup, so a restart resumes delivery without any action. A row
  that was being delivered when the process stopped is retried after its lease
  expires, which can deliver it twice in that single case (at-least-once).
- **One worker**: a single worker runs per process, and a row is claimed with a
  short lease in a conditional update, so two concurrent deliveries of the same
  row cannot happen.
- **If a local recipient copy fails** while a remote copy is queued, the SMTP
  client already has its `250`: the inline failure is recorded on the sent copy
  and the queued copies are still delivered.
- **Retry granularity is the domain**: if one recipient at a domain is accepted
  and another is rejected inside the same transaction, the retry can deliver the
  message twice to the recipient that was already accepted. Per-recipient state
  would be needed to avoid that, which this queue does not keep.
- **Rule-based forwarding** (a rule that forwards incoming mail) is unchanged and
  still synchronous.
- **No bounce is generated**: a permanently rejected message is marked failed in
  the sent folder and logged; it is not mailed back to the local sender.
- **Storage**: the `email_queue` table keeps delivered and failed rows as a
  delivery record, like the rest of the database. It lives in the same database
  file or server as the mailboxes, so the existing backup covers it. The table
  is bounded: a periodic cleanup (every hour, on the worker) deletes delivered
  rows older than **7 days** and finally failed rows older than **30 days**.
  Pending and in-flight rows are never deleted, whatever their age. Both
  windows are the `DeliveredRetention` and `FailedRetention` variables in
  `services/queue/retention.go`, and the delete is indexed by
  `(state, update_time)`, so it does not scan the table.
- **If you need bounces or per-recipient reports** — put a real MTA (Postfix,
  Exim) or a relay in front; iMail then delivers what that MTA hands it and
  queues only the remote recipients.


## Using the mailbox

The mailbox supports:

- Inbox, sent, drafts, junk, trash and custom folders.
- Search across subject and message text.
- Conversation mode with subject normalization across `Re:`, `Fwd:`, localized
  reply/forward prefixes and multiple folders.
- Reply, reply-all and forward with quoted original content.
- Attachments: inline images are served inline with the header
  `Content-Security-Policy: sandbox`; every other attachment is sent as
  `application/octet-stream` with `Content-Disposition: attachment` and
  `X-Content-Type-Options: nosniff`.
- Conversation pagination: five messages per page, then “Load more” or scroll
  to load the next page.
- Per-message actions under the date: **View source** and **Delete this
  message**. Source is the stored original MIME when the row has one (messages
  received over SMTP); sent messages and rows without stored MIME use a
  reconstructed header/body view.
- Per-mailbox quota display and settings.

Raw MIME is stored in the database for new SMTP messages so “View source” can
show the original headers, attachments and custom headers. This increases disk
usage; include the database in backups and monitor free space.

## Administration

Open the settings page (`#/settings` in the web client) from the mailbox or the
settings icon. The page is split into personal settings and administration.

### Personal settings

- **Password** — shows the current mailbox address and changes the password.
  The account display is on this page so it is visible on both desktop and
  mobile. Password changes revoke all existing sessions, including the current
  one; sign in again afterwards.
- **Folders** — create, rename and delete the user’s mail folders.
- **Rules** — match on sender, subject, recipients, text, HTML or content and
  read, move, delete or forward. Forward targets are validated as email
  addresses; move targets must belong to the same user.

### Administration (administrators only)

- **Users** — create, edit, disable and delete mailboxes. Each user has an
  account, display name, primary domain, password and quota in MiB. Account
  names are 1–20 characters, must begin with a letter or a digit, and may contain
  letters, digits, dots, underscores and hyphens; passwords are at least 8
  characters.
- **Security** — the server-wide protection switch. “Limit failed sign-ins” is
  on by default: five failed SMTP, IMAP or POP3 sign-ins within fifteen minutes
  refuse further attempts from the same source address for fifteen minutes.
  Accounts are a second, deliberately weaker control: a mailbox is only blocked
  when failures keep coming from several addresses (25 failures from at least
  three distinct sources within fifteen minutes), so one attacker cannot lock
  the owner out. The webmail and administrator logins are limited the same way.
  The switch is applied immediately, without a restart, and is saved in
  `config.json`. The same page shows the **outbox**: how many
  outbound messages are waiting for delivery to other servers and how many have
  finally failed, lists the queued rows and can retry a failed one or delete a
  finished one; the queue itself drains automatically.
- **Domains** — add or remove configured domains. A domain with users cannot be
  removed, and the primary domain cannot be removed. Removing a domain triggers
  a listener reload.
- **DNS** — show the records for a domain and copy individual rows or the whole
  table.
- **TLS Certificate** — opened from a row on the Domains page, so each domain
  can use its own certificate. Choose:
  - **HTTP-01** — requires inbound port 80 and a public hostname.
  - **Manual DNS-01** — the dialog shows the TXT records to add; issuance waits
    while you add them.
  - **Cloudflare DNS-01** — uses an API token with Zone.DNS edit permission.
  - **Upload** — paste or select a PEM certificate and private key.
  Shared certificates are marked as such; domains without their own
  certificate fall back to the shared one.

The administration console is intentionally small. There is no plugin or hook
system; all state is in the configuration file and database described below.

## Certificates

A certificate must cover every hostname a client uses. For a domain
`example.com`, that normally means:

```text
example.com
smtp.example.com
imap.example.com
pop.example.com
mail.example.com   (when it is the webDomain)
```

iMail supports three certificate flows:

1. **Global setup certificate** — selected in the setup wizard. It is used as
   the shared fallback and for the web listener when a domain has no dedicated
   certificate.
2. **Per-domain certificate** — issue or upload one from Domains → TLS. The
   certificate is stored under `config/ssl/domains/<domain>/` and selected by
   SNI for that domain, its `smtp.`, `imap.` and `pop.` names, and the
   `webDomain` when it is the domain itself or a subdomain of it.
3. **Automatic renewal** — HTTP-01 and Cloudflare DNS-01 certificates are
   checked daily and renewed when fewer than 30 days remain. Manual DNS-01 and
   uploaded certificates are not renewed automatically; watch their expiry in
   the UI.

The web UI and the mail listeners use the same certificate selection logic.
If a certificate does not include a hostname, clients will see a name mismatch
even if the certificate itself is valid.


## Configuration reference

The setup wizard writes `config/config.json`. Values that are normally managed
by the UI can also be edited while the container is stopped. Back up the file
before editing it; it is written atomically with mode `0600`.

```jsonc
{
  "logLevel": "info",              // info | debug | warn | error
  "domain": "example.com",         // primary mail domain
  "domains": ["example.com", "example.net"],
  "webDomain": "mail.example.com",
  "dkimPrivateKeyPath": "config/dkim/dkim.priv",
  "sslType": "0",                  // "0" HTTP-01, "1" upload, "2" DNS-01
  "SSLPrivateKeyPath": "config/ssl/private.key",
  "SSLPublicKeyPath": "config/ssl/public.crt",
  "dbType": "sqlite",              // sqlite | mysql | postgres
  "dbDSN": "./config/imail.db",
  "httpsEnabled": 0,               // 0/1 HTTPS, 2 HTTP-only
  "httpPort": 80,
  "httpsPort": 443,
  "isInit": true,
  "listenMode": "both",            // both | ipv4 | ipv6
  "egressMode": "both",            // family used for outbound delivery
  "tlsMinVersion": "1.2",
  "tlsMaxVersion": "1.3",
  "disableSNICheck": false,
  "serverHostnames": [],
  "dnsProvider": "",               // "cloudflare" or empty/manual
  "dnsApiToken": "",
  "domainCertificates": {},
  "defaultQuotaMB": 1024,          // 0 = unlimited
  "limitFailedSignins": true,      // false disables the failed sign-in limiter
  "spamFilterLevel": 0             // 0 off; 1 SPF+DKIM fail; 2 SPF fail; 3 DKIM fail
}
```

Notes:

- `SSLPrivateKeyPath`, `SSLPublicKeyPath` and `dkimPrivateKeyPath` may be
  absolute or relative; a relative path is resolved against the `IMAIL_ROOT`
  environment variable, or against the directory of the executable when
  `IMAIL_ROOT` is unset.
- `tlsMinVersion` and `tlsMaxVersion` accept `1.2` and `1.3`. Any other value
  falls back to `1.2` for the minimum and `1.3` for the maximum, and the two are
  swapped when the maximum is lower than the minimum.
- Two environment variables are read: `IMAIL_ROOT` (path resolution above) and
  `setup_port` (the port of the setup wizard's HTTP listener, overridable with
  the `-p` flag). Every other setting lives in `config.json`.
- `dnsApiToken` is a secret. It is redacted from startup logs, but it remains
  in `config.json`; protect that file and never commit it.
- `domainCertificates` stores per-domain mode/provider/token metadata. Use the
  Domains page rather than editing it by hand.
- MySQL and PostgreSQL take the connection string in `dbDSN` and the driver name
  in `dbType`; the schema is created and synchronized on startup. The default
  SQLite database is suitable for a single small server.
- `isInit` is set by the setup wizard. Do not set it manually before completing
  setup unless you are restoring an existing configuration.
- `listenMode` and `egressMode` are independent: a host may accept mail on both
  families while sending only over IPv4, for example. Leave both at `both`
  unless the host has a broken stack or a provider that blocks one family. On a
  host without IPv6 (or without IPv4) the configured family is downgraded
  automatically and the reason is logged at startup.

## Docker and operations

The included Compose file runs one container and mounts `./config`:

`docker-compose.yml` builds the image locally. `docker-compose.ghcr.yml` is the
same deployment but pulls `ghcr.io/noelbennettz/imail:latest` instead; use the
latter when you do not want a local Go/Node build.
`docker-compose.example.yml` is the same service with the ports, the volume and
the variable commented in place.

| Key | Meaning |
| --- | --- |
| `ports` | `"host address:host port:container port"`. 25 (SMTP), 465 (SMTPS), 587 (submission), 993 (IMAPS), 995 (POP3S) and 110 (POP3) must stay reachable from the internet for mail to work; 80 serves HTTP, ACME HTTP-01 and the redirect to HTTPS, 443 serves the webmail and admin console. Naming a host address (`127.0.0.1:443:443`) publishes that port on the loopback only — useful for 80/443 behind a TLS proxy, after setting `httpsEnabled` to `2` in `config.json` |
| `volumes` | `./config:/work/config` is the whole server state: configuration, SQLite database, DKIM key, certificates. A named volume behaves the same; a host directory is easier to back up |
| `environment: TZ` | Accepted but has no effect: iMail sets the process local zone to UTC, so log lines and stored timestamps are UTC and the browser renders them in the device's local time |
| `healthcheck` | Probes `/api/ping` on `127.0.0.1` and then `[::1]`, so a container with no IPv4 loopback still reports healthy |
| `image` / `build` | Pull `ghcr.io/noelbennettz/imail:latest`, or build from this checkout with `VERSION: internal` as the build argument |

The container's own network is a plain bridge and needs no IPv6 of its own:
listeners are published on the host, and the address family a listener uses is
chosen by `listenMode` in `config.json`, not by Docker.

```yaml
services:
  imail:
    build: .
    image: imail:internal
    restart: unless-stopped
    ports:
      - "25:25"
      - "80:80"
      - "110:110"
      - "443:443"
      - "465:465"
      - "587:587"
      - "993:993"
      - "995:995"
    volumes:
      - ./config:/work/config
```

The application stores everything it needs under `/work/config`: the JSON
configuration, SQLite database, DKIM key and TLS material. Keep that volume on
persistent storage.

The container health check calls `/api/ping`. While the HTTP listener answers,
the endpoint returns HTTP 200 with `{"data":"pong"}` — during setup as well as
after it, because the health check runs before the wizard is finished. A passing
probe therefore only shows that the HTTP listener responded. If the container is
healthy but mail is not working, check the mail listener logs and the host
firewall separately.

`TZ` changes neither log lines nor stored timestamps: iMail sets the process
local zone to UTC, and the browser renders dates in the device's local time.

### Removing it

State is the `./config` directory, not a Docker volume. What each command
removes:

| Command | Removes | Keeps |
| --- | --- | --- |
| `docker compose stop` | the running container | container, `./config`, image |
| `docker compose down` | the container and the compose network | `./config`, the image, the compose file |
| `docker compose down -v` | the above and any named volumes the file declares | `./config` — it is a bind mount, delete the directory yourself |
| `docker image rm imail:internal` (or `ghcr.io/noelbennettz/imail:latest`) | the image | — |
| `docker rm -f imail` | the container of a `docker run` deployment | `/srv/imail/config`, the image |

`rm -rf config/` deletes the configuration, the database, the DKIM key and every
certificate: the mailboxes and their mail are in that SQLite file, so it cannot
be undone. Copy the directory first ([Backups and upgrades](#backups-and-upgrades)).
The host firewall and DNS records are untouched by any of these commands.

### Updating

```bash
# local build:
docker compose build
docker compose up -d
docker compose logs -f imail
```

For the published GHCR Compose file:

```bash
docker compose -f docker-compose.ghcr.yml pull
docker compose -f docker-compose.ghcr.yml up -d
```

The workflow publishes `ghcr.io/noelbennettz/imail`, so the equivalent update
for a `docker run` deployment is:

```bash
docker pull ghcr.io/noelbennettz/imail:latest
docker stop imail && docker rm imail
# run the docker run command from "Run the published image" again
```

The `config/` volume is not touched by an image update. Existing MD5 password
hashes are upgraded to PBKDF2 on the next successful login, so no manual
password reset is required. Sessions are stored in the database and the cookie
is named `imail_session`, so an image update keeps users signed in.


## Backups and upgrades

Back up the whole `config/` directory. A consistent SQLite backup is easiest
when the container is stopped:

```bash
docker compose stop imail
tar -C . -czf imail-backup-$(date +%F).tar.gz config
docker compose start imail
```

For a live SQLite backup, use SQLite’s `.backup` command or copy while writes
are stopped; do not copy a database mid-write and expect a reliable restore.
The backup must include:

- `config/config.json`
- the SQLite file (`config/imail.db` by default) or your external database
- `config/dkim/`
- `config/ssl/` including `config/ssl/domains/<domain>/`

Restore by stopping the container, replacing `config/`, and starting it again.
The schema is synchronized on startup. Test a restore on a separate host
before relying on it.

## Security notes

How to report a vulnerability, which versions are supported and what is out of
scope is in [SECURITY.md](SECURITY.md). The list below is what the code does.

- Passwords are stored as salted PBKDF2-HMAC-SHA256 hashes (210,000
  iterations). Password changes revoke existing sessions. Minimum length is 8
  characters.
- Failed sign-ins are limited while `limitFailedSignins` is on (the default,
  including for `config.json` files written before the setting existed). The
  source address is the primary control: five failures within fifteen minutes
  block that address for fifteen minutes in SMTP AUTH, IMAP, POP3 and the
  `/api/login` used by webmail and the administrator console. A blocked attempt
  gets the protocol's ordinary authentication failure (`535`,
  `NO [AUTHENTICATIONFAILED]`, `-ERR`, a generic `400` for the web login), so a
  block never tells an attacker whether the account exists, and the check runs
  before the password hash is computed. The address counter is shared by every
  protocol, so switching protocol does not reset a block.
- Accounts are a second, weaker control against a slow distributed guess: a
  mailbox is blocked for fifteen minutes only after 25 failures within fifteen
  minutes that came from at least three distinct addresses. One source alone can
  therefore never lock an account out. Account counters are separate for mail,
  webmail and the administrator console, so a spray at a user mailbox cannot
  block an administrator's login.
- There is deliberately no server-wide ceiling that refuses attempts: a
  distributed spray must not be able to shut legitimate logins out. Password
  hashing is bounded instead — at most four PBKDF2 verifications run at the same
  time, so a spray can delay a correct login but never make it fail. Failures are
  counted globally and produce one warning line per minute when 100 are reached;
  that counter is for alerting only and never blocks anyone. The in-memory table
  is capped at 4096 keys. Counters are not persisted: restarting the server
  clears them. Administrators can turn the switch off on the Security page; it
  applies immediately.
- Multi-domain deployments require a full `account@domain` login for web,
  SMTP, IMAP and POP3. The account namespace is global: `account` is a unique
  column, so duplicate local parts across domains are not possible.
- TLS listeners require a known SNI by default. Keep `disableSNICheck` off.
- Plaintext SMTP and POP3 greetings use a generic server name (`imail`) and do
  not disclose any configured mail domain.
- Outbound SMTP tries port 25 with STARTTLS, then 587 with STARTTLS, then 465
  with implicit TLS, and verifies the MX certificate against the MX hostname. It
  never falls back to an unverified TLS session or to plaintext; a delivery that
  cannot establish verified TLS is reported as a delivery failure.
- Session cookies are `HttpOnly` and `SameSite=Lax`; `Secure` is enabled when
  HTTPS is enabled. Do not run an HTTPS deployment through an untrusted proxy.
- The setup wizard is available before first initialization. Complete it on a
  trusted network and do not expose an unconfigured instance to the internet.
  The wizard's API requires the one-time setup token printed in the container
  log; the token lives in memory, is compared in constant time, and is dropped
  once initialization succeeds. It prevents a stranger who reaches the port
  first from finishing setup, but it does not encrypt the administrator password
  in transit — use an SSH tunnel, a VPN or the machine's console.
- “View source” stores and displays the original MIME for messages received over
  SMTP. That can include full headers and attachments; protect the database and
  backups accordingly.
- Run the container behind a firewall and expose only the ports you need. If
  you terminate TLS at a reverse proxy, forward the original hostname and
  `/.well-known/acme-challenge/` correctly.

## Troubleshooting

### I can log in but cannot send or receive mail

Check the provider’s outbound TCP 25 policy first. Many VPS providers block 25
until you request an unblock. Then verify:

```bash
dig +short MX example.com
dig +short A smtp.example.com
dig +short TXT _dmarc.example.com
nc -vz <server-ip> 25
```

Also check that the host firewall and Docker port mapping allow the mail ports,
and that reverse DNS (PTR) for the public address points at your SMTP hostname.
SPF, DKIM and DMARC records must match the domain you send from.

### TLS handshake fails or clients report a name mismatch

The requested hostname must be in the certificate SAN list and in the server’s
known hostname set. Check the certificate from the client side:

```bash
openssl s_client -connect smtp.example.com:465 -servername smtp.example.com
openssl s_client -connect imap.example.com:993 -servername imap.example.com
```

For a domain with its own certificate, issue or upload it on Domains → TLS. If
you use a reverse proxy, preserve SNI and the public hostname; adding the name
to `serverHostnames` is safer than disabling SNI checks.

### ACME issuance fails

HTTP-01 requires port 80 to reach iMail (or a proxy that forwards
`/.well-known/acme-challenge/`). DNS-01 requires either the manually displayed
TXT records or a Cloudflare token with Zone.DNS edit permission. Wait for DNS
propagation and check the status message in the TLS dialog. Do not retry in a
tight loop; ACME providers rate-limit failures.

### Login says the domain must be specified

This is expected with more than one configured domain. Use the full address
(`alice@example.com`). With one domain, either the bare account or the full
address works. New passwords shorter than 8 characters are rejected.

### Times look shifted

iMail stores UTC and the browser converts timestamps to the device timezone.
Check the client’s timezone rather than the server. Databases carried over from
builds that wrote timestamps in a fixed server zone can contain rows offset by
that zone; those rows need a one-time correction after migrating.

### POP3 says TLS is required

Port 110 accepts a generic plaintext greeting but authentication requires
`STLS` first. Use POP3S on 995, or configure the client to issue `STLS` on 110.


### Mail works on IPv4 but not IPv6 (or the reverse)

Check the effective family in the startup warning (`listenMode "ipv6" is not
available on this host, listening on "both" instead`, and the matching
`egressMode` line). Verify the host actually has the address and a route:

```bash
ip -6 addr show scope global
ip -6 route show default
curl -6 -sS https://api6.ipify.org     # outbound IPv6
curl -4 -sS https://api.ipify.org      # outbound IPv4
```

Then check DNS: an A record is useless for an IPv6-only host and an AAAA record
is useless for an IPv4-only host. Domains → DNS records shows the addresses the
server actually detected from its own egress, and the container healthcheck
probes both loopbacks, so a passing healthcheck does not tell you which family
is reachable from outside. Test both explicitly, for example
`nc -6 -vz <server-ipv6> 25` and `nc -4 -vz <server-ipv4> 25`.

### Docker cannot bind a port

Rootless Docker cannot bind ports below 1024. Publish high host ports and map
them to the container’s standard ports, for example `-p 8080:80 -p 18443:443`;
this changes host reachability, not the ports iMail uses inside the container.

## Development

Requirements: Go 1.24 or newer and Node.js/Yarn for the frontend.

```bash
make build       # frontend + server binaries
make docker      # build imail:internal
make test        # Go tests; binding ports below 1024 requires privileges
cd fe && yarn dev
```

The Go server is in `server/`; the Vue 3 frontend is in `fe/`. The frontend is
embedded into the Go binary for production, so the final image needs no Node.js
runtime. The repository vendors Go dependencies and builds with `-mod=vendor`.
`make test` runs with `setup_port=17888` and `IMAIL_ROOT` pointed at `server/`;
`make test_mysql` and `make test_postgres` run the root integration suite
against a server called `mysql` or `postgres`.

The queue and webmail tests can also run against a real server database. Point
them at one and they use it instead of their throwaway SQLite file:

```bash
IMAIL_TEST_DB=mysql \
IMAIL_TEST_DSN='root:secret@tcp(127.0.0.1:3306)/imail?parseTime=True&loc=UTC' \
  go test -p 1 ./services/queue/ ./controllers/email/
IMAIL_TEST_DB=postgres \
IMAIL_TEST_DSN='postgres://postgres:secret@127.0.0.1:5432/imail?sslmode=disable' \
  go test -p 1 ./services/queue/ ./controllers/email/
```

The CI `Tests` workflow runs exactly that against MySQL 8.4 and PostgreSQL 16
service containers. Use `-p 1`: the tests rebuild the schema once per process,
so two packages must not share the database at the same time.

Format Go code with gofumpt (`gofumpt -l -w .`). The root integration test
starts the real SMTP, POP3 and IMAP listeners on 25, 110, 465, 587, 993 and 995
and the HTTP listener on `setup_port`, so it needs privileges to bind the mail
ports; package-level tests run without them. `go vet ./...` should be clean.
[CONTRIBUTING.md](CONTRIBUTING.md) has the patch, test and DCO sign-off rules.

