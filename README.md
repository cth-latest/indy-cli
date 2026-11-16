<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset=".github/logo-dark.svg">
    <img src=".github/logo-light.svg" alt="Indy" width="160">
  </picture>
</p>

# indy-cli

Unofficial command-line client for [Indy](https://app.indy.fr), the French accounting app. It is built for scripts and AI agents: every command prints JSON, so an agent can list transactions, categorize them, set VAT, attach receipts and export accounting documents.

> This project is not affiliated with Indy. It uses the web app's internal API, which can change without notice. Use it on your own account, at your own risk.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/cth-latest/indy-cli/main/install.sh | sh
```

This installs the latest release to `~/.local/bin` on macOS and Linux. Set `INDY_INSTALL_DIR` to install somewhere else.

To update later, run `indy upgrade`. `indy upgrade --check` only tells you whether a newer release exists.

To build from source instead, you need Go:

```sh
go install github.com/cth-latest/indy-cli@latest
```

## Log in

```sh
indy login
indy whoami
```

`indy login` opens a Chrome window. Log in as usual, wait for your Indy dashboard, then press Enter in the terminal. The window closes and the session is saved.

- **Chrome is required.** The login uses its own Chrome profile, so your normal Chrome is untouched. Set `INDY_CHROME` if the binary is in an unusual place.
- **Without Chrome**, copy the `__Http-sid` cookie from the DevTools of app.indy.fr and run `indy login --cookie <value>`.
- **Session file:** `~/Library/Application Support/indy/session.json` on macOS, `~/.config/indy/session.json` on Linux, readable only by you. It lasts about a year. `indy logout` removes it.
- **Why a browser:** Indy protects its login with a Cloudflare Turnstile captcha, which only a real browser can pass.

## Use

```sh
indy tx list --from 2026-01-01 --has-receipts no --direction expense --all
indy tx categories                          # categories and their account numbers
indy tx categorize TX_ID SUB_ID 613600
indy tx vat TX_ID SUB_ID --rate 2000
indy receipt upload ./facture.pdf --tx TX_ID
indy vat declarations --year 2026
indy doc fec 2026
indy help                                   # every command
```

| Group | What it covers |
|---|---|
| `tx` | List and filter transactions, categorize, set VAT, verify, annotate, change date, delete and restore |
| `receipt` | Upload receipts, attach them to transactions, find matches |
| `invoice` | List invoices, download PDFs, link them to transactions |
| `vat` | VAT configuration, declarations, periods and declaration lines |
| `doc` | Balance sheet, profit and loss, trial balance, FEC, journal, exports |
| `bank`, `activities`, `dashboard`, `closing` | Bank accounts and connections, activities, dashboard, closing status |
| `api` | Raw call to any endpoint the CLI does not wrap: `indy api GET /transactions/to-categorize-count` |

Things to know:

- **Units:** amounts are in cents. VAT rates are in basis points, so `2000` means 20%.
- **Output:** JSON on stdout. Errors go to stderr with exit code 1.
- **Fewer tokens:** add `--toon` to any command to get [TOON](https://toonformat.dev/) instead of JSON.
- **Environment:** `INDY_SESSION` (the session cookie) and `INDY_ORG_ID` override the saved session; `INDY_SESSION_FILE` changes where it is stored.

## Develop

```sh
go build -o indy . && go test ./...
```

Pushing a tag such as `v0.2.0` builds the macOS and Linux binaries and publishes a GitHub release.
