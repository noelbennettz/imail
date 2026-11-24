# Contributing to iMail

Thanks for taking the time to send a patch. This project is small and reviewed
by one person in their spare time, so a focused change with tests and a clear
commit message goes a long way.

## What lives where

| Path | Contents |
| --- | --- |
| `server/` | Go backend: mail listeners, HTTP APIs, setup wizard, embedded web client. Dependencies are vendored. |
| `fe/` | Vue 3 client, built with Vite and Yarn. The build output is embedded into the Go binary. |
| [`docs/`](https://github.com/noelbennettz/noelbennettz.github.io/tree/main/docs/imail) | Not in this repository: the project site lives in `noelbennettz.github.io` under `docs/imail/` (`index.html` in English, `zh.html` in Traditional Chinese), published at <https://noelbennettz.github.io/imail/>. |
| `README.md`, `DOCS.md` | User documentation, in English. |

## Requirements

- Go 1.24 or newer (`go version`).
- Node.js 22 and Yarn 1 for the frontend. `fe/yarn.lock` is committed, so use
  `yarn install --frozen-lockfile`.
- Optional: Docker with Compose to build the image, and Chrome or Chromium for
  the frontend tests (`CHROME_BIN` may point at the binary).

## Build and test

```bash
make build          # frontend, then the server binaries
make docker         # build imail:internal locally
make test           # Go tests; the integration test binds mail ports and needs privileges
cd fe && yarn test  # frontend tests, needs Chrome/Chromium
```

`make test` sets `setup_port=17888` and `IMAIL_ROOT` for the integration test,
which starts the real listeners. Package-level tests (`cd server && go test
./...`) run without privileges; only the root package needs to bind ports below
1024. `make test_mysql` and `make test_postgres` run the same suite against
those databases.

On a fresh clone the Go packages need two fixtures that CI creates: the embedded
web client at `server/listen/http_server/dist` (otherwise `go:embed all:dist`
cannot compile) and a throwaway key pair at
`server/config/ssl/{private.key,public.crt}` (the IMAP and root suites load it).
Create both once:

```bash
(cd fe && yarn install --frozen-lockfile && yarn build)
rm -rf server/listen/http_server/dist
cp -r fe/dist server/listen/http_server/dist
mkdir -p server/config/ssl
openssl req -x509 -newkey rsa:2048 -nodes -days 1 \
  -keyout server/config/ssl/private.key \
  -out server/config/ssl/public.crt -subj "/CN=localhost"
```

`server/config/ssl` is gitignored; never point it at a real deployment's
certificates.

Before sending a change:

```bash
gofumpt -l .        # add -w to format; vendor/ is skipped
cd server && go vet ./...
cd fe && yarn build
```

## DCO: sign every commit

Contributions are accepted under the
[Developer Certificate of Origin](https://developercertificate.org/) (DCO) 1.1.
There is no CLA and nothing to sign up for: you add one trailer line to each
commit message.

```bash
git commit -s -m "Fix the IMAP IDLE timeout"
```

`-s` appends:

```
Signed-off-by: Your Name <your@email>
```

The sign-off must use the same name and email as the commit author. By adding it
you certify that you wrote the change, or that you have the right to submit it
under the project licence (AGPL-3.0).

Missed one? Fix it before the pull request is merged:

```bash
git commit --amend -s                       # the last commit
git rebase --signoff origin/main            # every commit of your branch
git push --force-with-lease                 # your own branch only
```

`.github/workflows/dco.yml` checks every commit of a pull request with
`scripts/check-dco.sh`. Commits authored by bots are skipped, and merge commits
are not checked. The same script works locally:

```bash
scripts/check-dco.sh origin/main HEAD
```

## Pull requests

- Keep the change focused; unrelated cleanups belong in their own pull request.
- Add or update tests for behaviour you change. Tests must use fresh temporary
  directories and must not touch `config/` of a running deployment, connect to
  external mail servers or send mail.
- Write commit messages and code comments in English.
- User-facing behaviour goes into `README.md` or `DOCS.md` (English), and the
  matching page of the project site (English and Traditional Chinese, in the
  [`noelbennettz.github.io`](https://github.com/noelbennettz/noelbennettz.github.io)
  repository under `docs/imail/`) stays in sync.
- A new dependency needs its licence recorded in `THIRD_PARTY_NOTICES.md` with an
  exact version, and it has to pass the licence check that CI runs for Go and
  front-end dependencies. If the check rejects a licence, the pull request is
  the place to discuss it.
- No telemetry, no external network calls at build or run time, and no assets
  loaded from a CDN — the client is served entirely from the binary.
