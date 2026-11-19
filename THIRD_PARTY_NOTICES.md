# Third-party notices — iMail

iMail itself is licensed under the **GNU Affero General Public License v3.0**
(see [LICENSE](LICENSE)). It bundles or links the components listed below. Each
one keeps its own licence; the full text ships with the code, either next to the
vendored module under `server/vendor/<module>/` or in the file named in the
table. None of the vendored code has been modified, so no notice of changes is
due.

Summary of the linked components: Apache-2.0 ×3, BSD-2-Clause ×1,
BSD-3-Clause ×21, MIT ×22, MPL-2.0 ×1. The container image additionally carries
separate Alpine system packages with their own licences, listed under
[Container base image and system packages](#container-base-image-and-system-packages);
none of them is linked into the iMail binary.

Two checks in CI keep this file honest: `go-licenses check` over the Go modules
and `license-checker` over the front-end production dependencies of `fe/`, both
in the `licences` workflow.

## Go modules vendored into the server

`server/vendor/` is committed, so the exact source this binary was built from —
and every licence text below — is in the repository.

| Module | Version | Licence | Licence text |
| --- | --- | --- | --- |
| `github.com/go-jose/go-jose/v4` | v4.1.0 | Apache-2.0 | `server/vendor/github.com/go-jose/go-jose/v4/LICENSE` |
| `github.com/modern-go/concurrent` | v0.0.0-20180306012644-bacd9c7ef1dd | Apache-2.0 | `server/vendor/github.com/modern-go/concurrent/LICENSE` |
| `github.com/modern-go/reflect2` | v1.0.2 | Apache-2.0 | `server/vendor/github.com/modern-go/reflect2/LICENSE` |
| `github.com/syndtr/goleveldb` | v1.0.0 | BSD-2-Clause | `server/vendor/github.com/syndtr/goleveldb/LICENSE` |
| `filippo.io/edwards25519` | v1.1.0 | BSD-3-Clause | `server/vendor/filippo.io/edwards25519/LICENSE` |
| `github.com/golang/snappy` | v1.0.0 | BSD-3-Clause | `server/vendor/github.com/golang/snappy/LICENSE` |
| `github.com/google/uuid` | v1.6.0 | BSD-3-Clause | `server/vendor/github.com/google/uuid/LICENSE` |
| `github.com/gorilla/css` | v1.0.1 | BSD-3-Clause | `server/vendor/github.com/gorilla/css/LICENSE` |
| `github.com/microcosm-cc/bluemonday` | v1.0.27 | BSD-3-Clause | `server/vendor/github.com/microcosm-cc/bluemonday/LICENSE.md` |
| `github.com/miekg/dns` | v1.1.65 | BSD-3-Clause | `server/vendor/github.com/miekg/dns/LICENSE` |
| `github.com/remyoudompheng/bigfft` | v0.0.0-20230129092748-24d4a6f8daec | BSD-3-Clause | `server/vendor/github.com/remyoudompheng/bigfft/LICENSE` |
| `golang.org/x/crypto` | v0.43.0 | BSD-3-Clause | `server/vendor/golang.org/x/crypto/LICENSE` |
| `golang.org/x/exp` | v0.0.0-20251023183803-a4bb9ffd2546 | BSD-3-Clause | `server/vendor/golang.org/x/exp/LICENSE` |
| `golang.org/x/mod` | v0.29.0 | BSD-3-Clause | `server/vendor/golang.org/x/mod/LICENSE` |
| `golang.org/x/net` | v0.46.0 | BSD-3-Clause | `server/vendor/golang.org/x/net/LICENSE` |
| `golang.org/x/sync` | v0.17.0 | BSD-3-Clause | `server/vendor/golang.org/x/sync/LICENSE` |
| `golang.org/x/sys` | v0.37.0 | BSD-3-Clause | `server/vendor/golang.org/x/sys/LICENSE` |
| `golang.org/x/text` | v0.30.0 | BSD-3-Clause | `server/vendor/golang.org/x/text/LICENSE` |
| `golang.org/x/tools` | v0.38.0 | BSD-3-Clause | `server/vendor/golang.org/x/tools/LICENSE` |
| `modernc.org/libc` | v1.67.6 | BSD-3-Clause | `server/vendor/modernc.org/libc/LICENSE` (plus `LICENSE-3RD-PARTY.md` for the C sources it carries) |
| `modernc.org/mathutil` | v1.7.1 | BSD-3-Clause | `server/vendor/modernc.org/mathutil/LICENSE` |
| `modernc.org/memory` | v1.11.0 | BSD-3-Clause | `server/vendor/modernc.org/memory/LICENSE` |
| `modernc.org/sqlite` | v1.46.1 | BSD-3-Clause | `server/vendor/modernc.org/sqlite/LICENSE` (SQLite itself is public domain, see `SQLITE-LICENSE`) |
| `xorm.io/builder` | v0.3.13 | BSD-3-Clause | `server/vendor/xorm.io/builder/LICENSE` |
| `xorm.io/xorm` | v1.3.9 | BSD-3-Clause | `server/vendor/xorm.io/xorm/LICENSE` |
| `github.com/alexedwards/scs/mysqlstore` | v0.0.0-20250417082927-ab20b3feb5e9 | MIT | `server/vendor/github.com/alexedwards/scs/mysqlstore/LICENSE` |
| `github.com/alexedwards/scs/postgresstore` | v0.0.0-20250417082927-ab20b3feb5e9 | MIT | `server/vendor/github.com/alexedwards/scs/postgresstore/LICENSE` |
| `github.com/alexedwards/scs/sqlite3store` | v0.0.0-20250417082927-ab20b3feb5e9 | MIT | `server/vendor/github.com/alexedwards/scs/sqlite3store/LICENSE` |
| `github.com/alexedwards/scs/v2` | v2.8.0 | MIT | `server/vendor/github.com/alexedwards/scs/v2/LICENSE` |
| `github.com/aymerick/douceur` | v0.2.0 | MIT | `server/vendor/github.com/aymerick/douceur/LICENSE` |
| `github.com/cenkalti/backoff/v4` | v4.3.0 | MIT | `server/vendor/github.com/cenkalti/backoff/v4/LICENSE` |
| `github.com/dlclark/regexp2` | v1.11.5 | MIT | `server/vendor/github.com/dlclark/regexp2/LICENSE` |
| `github.com/dustin/go-humanize` | v1.0.1 | MIT | `server/vendor/github.com/dustin/go-humanize/LICENSE` |
| `github.com/emersion/go-imap/v2` | v2.0.0-beta.5 | MIT | `server/vendor/github.com/emersion/go-imap/v2/LICENSE` |
| `github.com/emersion/go-message` | v0.18.2 | MIT | `server/vendor/github.com/emersion/go-message/LICENSE` |
| `github.com/emersion/go-msgauth` | v0.6.8 | MIT | `server/vendor/github.com/emersion/go-msgauth/LICENSE` |
| `github.com/emersion/go-sasl` | v0.0.0-20241020182733-b788ff22d5a6 | MIT | `server/vendor/github.com/emersion/go-sasl/LICENSE` |
| `github.com/emersion/go-smtp` | v0.21.3 | MIT | `server/vendor/github.com/emersion/go-smtp/LICENSE` |
| `github.com/go-acme/lego/v4` | v4.23.1 | MIT | `server/vendor/github.com/go-acme/lego/v4/LICENSE` |
| `github.com/goccy/go-json` | v0.10.5 | MIT | `server/vendor/github.com/goccy/go-json/LICENSE` |
| `github.com/json-iterator/go` | v1.1.12 | MIT | `server/vendor/github.com/json-iterator/go/LICENSE` |
| `github.com/lib/pq` | v1.10.9 | MIT | `server/vendor/github.com/lib/pq/LICENSE.md` |
| `github.com/mattn/go-isatty` | v0.0.20 | MIT | `server/vendor/github.com/mattn/go-isatty/LICENSE` |
| `github.com/mileusna/spf` | v0.9.5 | MIT | `server/vendor/github.com/mileusna/spf/LICENSE.md` |
| `github.com/ncruces/go-strftime` | v1.0.0 | MIT | `server/vendor/github.com/ncruces/go-strftime/LICENSE` |
| `github.com/sirupsen/logrus` | v1.9.3 | MIT | `server/vendor/github.com/sirupsen/logrus/LICENSE` |
| `github.com/spf13/cast` | v1.7.1 | MIT | `server/vendor/github.com/spf13/cast/LICENSE` |
| `github.com/go-sql-driver/mysql` | v1.9.2 | MPL-2.0 | `server/vendor/github.com/go-sql-driver/mysql/LICENSE` |

### What the copyleft entries mean here

- **MPL-2.0** (`go-sql-driver/mysql`) is file-level copyleft: the unmodified
  files may be distributed alongside AGPL-3.0 code, but any file you patch has to
  stay under MPL-2.0 and its source has to be available. iMail does not patch it.
- The **Apache-2.0** modules ship no `NOTICE` file, so section 4(d) adds nothing
  beyond keeping the licence text and this notice. Their patent grant in
  section 3 still applies to every user of iMail.
- Four requirements in `server/vendor/modules.txt` have **no packages vendored**
  (`github.com/nxadm/tail`, `github.com/onsi/ginkgo`, `github.com/onsi/gomega`,
  `github.com/rogpeppe/go-internal`). They are indirect test-only modules and no
  code from them is distributed.

## Front end

`fe/` builds Vue 3, Vue Router, Pinia, Element Plus, `@element-plus/icons-vue`,
axios, `@wangeditor/editor`, `@wangeditor/editor-for-vue` and `vue-icons-plus`
into `server/listen/http_server/dist`. Their sources and licences are resolved
from npm at build time and are not vendored here. The versions are the ones
`fe/yarn.lock` pins; the licence column is what `license-checker` reports.

| Package | Version | Licence |
| --- | --- | --- |
| `@element-plus/icons-vue` | 2.3.1 | MIT |
| `@wangeditor/editor` | 5.1.23 | MIT |
| `@wangeditor/editor-for-vue` | 5.1.12 | MIT |
| `axios` | 1.8.4 | MIT |
| `element-plus` | 2.9.7 | MIT |
| `pinia` | 2.3.1 | MIT |
| `vue` | 3.5.13 | MIT |
| `vue-icons-plus` | 0.1.8 | MIT |
| `vue-router` | 4.5.0 | MIT |

The transitive production dependencies — 126 packages at the time of writing,
MIT, ISC, BSD-2-Clause and BSD-3-Clause — are covered by the same check rather
than listed one by one.

`fe/src/assets/fonts/` carries the Inter webfont under the **SIL Open Font
License 1.1**; the licence text is `fe/src/assets/fonts/OFL.txt`.

## Source files copied into the tree

- `server/utils/utf7/` — MIT, © The Go-IMAP Authors, Proton Technologies AG and
  Simon Ser. Licence: `server/utils/utf7/LICENSE`.
- `server/third_party/gopop/` — MIT, © 木木的木头. Licence:
  `server/third_party/gopop/LICENSE`.
- `server/utils/smtp/smtp.go` — derived from the Go standard library's
  `net/smtp`, BSD-3-Clause, © 2010 The Go Authors; the original header is kept
  in the file and the licence text is `server/utils/smtp/LICENSE`.

## Container base image and system packages

The runtime image is `alpine:3.21` plus the packages `apk add` installs in the
`Dockerfile` and their dependencies. They are separate programs in the same
image, not linked into the iMail binary: the Go build is static
(`CGO_ENABLED=0`) and links only the modules listed above, and the Go client is
served from files embedded in that binary. Their licences are their own, and
Alpine does not install the licence texts — they are in the package sources at
<https://git.alpinelinux.org/aports/tree/main/> and on the upstream sites
below.

Versions are the ones `apk` resolved when this file was last updated; the
licence column is the `L:` field of the Alpine package index for v3.21.

| Package | Version | Licence | Upstream |
| --- | --- | --- | --- |
| `alpine-baselayout`, `alpine-baselayout-data` | 3.6.8-r1 | GPL-2.0-only | <https://git.alpinelinux.org/aports/tree/main/alpine-baselayout> |
| `alpine-keys` | 2.5-r0 | MIT | <https://alpinelinux.org> |
| `alpine-release` | 3.21.10-r0 | MIT | <https://alpinelinux.org> |
| `apk-tools` | 2.14.13-r0 | GPL-2.0-only | <https://gitlab.alpinelinux.org/alpine/apk-tools> |
| `busybox`, `busybox-binsh` | 1.37.0-r14 | GPL-2.0-only | <https://busybox.net/> |
| `ca-certificates`, `ca-certificates-bundle` | 20261020-r0 | MPL-2.0 AND MIT | <https://www.mozilla.org/en-US/about/governance/policies/security-group/certs/> |
| `libcrypto3`, `libssl3`, `openssl` | 3.3.7-r2 | Apache-2.0 | <https://www.openssl.org/> |
| `libidn2` | 2.3.7-r0 | GPL-2.0-or-later OR LGPL-3.0-or-later | <https://www.gnu.org/software/libidn#libidn2> |
| `libunistring` | 1.2-r0 | GPL-2.0-or-later OR LGPL-3.0-or-later | <https://www.gnu.org/software/libunistring/> |
| `musl` | 1.2.5-r12 | MIT | <https://musl.libc.org/> |
| `musl-utils` | 1.2.5-r12 | MIT AND BSD-2-Clause AND GPL-2.0-or-later | <https://musl.libc.org/> |
| `pcre2` | 10.43-r1 | BSD-3-Clause | <https://pcre.org/> |
| `scanelf` | 1.3.8-r1 | GPL-2.0-only | <https://wiki.gentoo.org/wiki/Hardened/PaX_Utilities> |
| `ssl_client` | 1.37.0-r14 | GPL-2.0-only | <https://busybox.net/> |
| `tzdata` | 2026f-r0 | Public-Domain | <https://www.iana.org/time-zones> |
| `wget` | 1.25.0-r0 | GPL-3.0-or-later WITH OpenSSL-Exception | <https://www.gnu.org/software/wget/wget.html> |
| `zlib` | 1.4-r0 | Zlib | <https://zlib.net/> |

Nothing here is GPL-2.0-only code linked into the iMail program: `busybox`,
`apk-tools`, `alpine-baselayout`, `scanelf` and `ssl_client` are executables
that ship beside it. The image also carries `LICENSE` and this file in
`/usr/share/doc/imail/`, so whoever receives the image gets them with the
binary.
