# iMail — a self-hosted mail server for your own domains

> Applies to the current `main`, published as `ghcr.io/noelbennettz/imail:latest`
> (also tagged `main` and `sha-<commit>`). Documentation for an older release is
> kept under `docs/versions/<tag>/`.

**Status: beta** — incompatible changes between versions are possible, so back
up `config/` before you upgrade.

[![Website](https://img.shields.io/badge/website-noelbennettz.github.io%2Fimail-4f8dff)](https://noelbennettz.github.io/imail/)
[![Docker](https://img.shields.io/badge/Docker-compose-2496ED?logo=docker&logoColor=white)](docker-compose.yml)
[![Container image](https://img.shields.io/badge/ghcr.io-imail-blue?logo=github)](https://github.com/noelbennettz/imail/pkgs/container/imail)
[![License](https://img.shields.io/badge/License-AGPL--3.0-blue)](LICENSE)

iMail serves SMTP, submission, SMTPS, IMAPS, POP3 and POP3S for one or more
domains you control, and ships a Material 3 web mailbox and administration
console from a single Go binary with the Vue 3 client embedded, so production
needs no nginx, PHP, Node runtime or external asset CDN. It keeps all state —
configuration, SQLite database, DKIM key and TLS certificates — in one `config/`
volume, which makes a backup a directory copy; MySQL and PostgreSQL are
supported for larger deployments. iMail is licensed under AGPL-3.0.

## Requirements

- Docker with Compose, or Go 1.24+ to build and run the binary yourself.
- A public IPv4 and/or IPv6 address.
- A domain whose DNS you control.
- Inbound TCP ports **25, 80, 110, 443, 465, 587, 993 and 995** reachable from
  the internet (at least the ports your clients use), plus outbound TCP 25 for
  delivery to other mail servers.
- Port 80 available for ACME HTTP-01, unless you use DNS-01 or upload a
  certificate.

Many VPS and residential providers block port 25. Mail from other servers
always arrives on port 25 — an MX record carries no port — so if inbound 25 is
blocked, remote mail cannot reach iMail directly; the working setup is a relay
or a second host that accepts on 25 and forwards. Your own users can submit on
587 regardless. Outbound delivery to other mail servers also needs outbound 25,
or a relay you configure yourself; check with your provider before debugging the
application.

## Quick start

The published image is the fastest path and needs no build:

```bash
mkdir -p config
docker compose -f docker-compose.ghcr.yml pull
docker compose -f docker-compose.ghcr.yml up -d
docker compose -f docker-compose.ghcr.yml logs -f imail
```

To build from this checkout instead — this compiles the Vue frontend and the Go
binary, stays silent for several minutes and needs more memory than a 1 GB VPS
comfortably has:

```bash
git clone https://github.com/noelbennettz/imail.git
cd imail
mkdir -p config
docker compose up -d --build
docker compose logs -f imail
```

Open `http://<server-ip>/` and complete the setup wizard (database,
administrator, domains, DNS records, TLS). The first screen asks for the setup
token: the server prints it at startup, in a banner that contains the line
`Read it again with: docker compose logs imail`. Copy the 24-character token
from `docker compose logs imail` into that screen; every later wizard request
carries it as the `X-Setup-Token` header. The token is generated only while the
deployment is uninitialised, lives in memory, never touches `config/`, and is
discarded when setup finishes.

The wizard exists only before the first successful initialization, so finish it
on a trusted network. It is served over plain HTTP — TLS is configured at the
end of it — so the token stops a stranger from completing setup first, but it
does not encrypt the administrator password on the wire; use an SSH tunnel, a
VPN, or the machine's console for the first run.

Check it came up: `docker compose ps` shows `Up … (healthy)` after the
45-second start period, `curl -s http://127.0.0.1/api/ping` answers
`{"errorNo":0,"errorMsg":"","data":"pong"}`, and the browser shows the setup
wizard (or the login page once it is finished).

Removing it later: `docker compose down` removes the container and keeps
`./config`; `docker compose down -v` also removes named volumes — with the
shipped Compose files state is a bind mount, so `./config` stays until you
delete the directory. [DOCS.md](DOCS.md#removing-it) has the details, including
what `docker run` leaves behind.

## Security

Report a vulnerability through GitHub's private reporting: **Security** →
**Report a vulnerability** on <https://github.com/noelbennettz/imail>. If that
form is unavailable, open an issue titled `Security contact request` that says
nothing about the technical details. [SECURITY.md](SECURITY.md) lists the
supported versions, what is in and out of scope, and what to expect. The
[security notes in DOCS.md](DOCS.md#security-notes) describe the behaviour you
can check against the code.

## Documentation

| File | Contents |
| --- | --- |
| [CONTRIBUTING.md](CONTRIBUTING.md) | Development setup, tests, and the sign-off every commit needs (DCO). |
| [SECURITY.md](SECURITY.md) | Supported versions, how to report a vulnerability, and what is in scope. |
| [DOCS.md](DOCS.md) | Features, installation detail, DNS records, ports and TLS, mailbox and administration, certificates, configuration reference, operations, backups, security notes, troubleshooting, development. |
| [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) | Licence of every vendored Go module, front-end package and copied source file. |
| [`docs/`](docs/) | Project site in English and Traditional Chinese, published at <https://noelbennettz.github.io/imail/> |
| [docker-compose.example.yml](docker-compose.example.yml) | Every compose port, volume and variable, commented in place. |

## Contributing

Contributions are welcome: [CONTRIBUTING.md](CONTRIBUTING.md) covers the build,
the tests and the `Signed-off-by` line every commit needs under the
[Developer Certificate of Origin](https://developercertificate.org/).

## Licence

iMail is licensed under the GNU Affero General Public License v3.0; see
[LICENSE](LICENSE). Because iMail is a network service, section 13 applies: the
login page and the sidebar carry a source-code link, and every release is built
from this public repository, so anyone using the web interface is offered the
source of the version they are running — point that link at your own source if
you run a modified iMail. Vendored components keep their own licences, listed in
[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md); the published image ships both
that file and LICENSE under `/usr/share/doc/imail/`.
