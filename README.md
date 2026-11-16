# indy-cli

Unofficial CLI for [Indy](https://app.indy.fr), built for scripts and AI agents. It is not affiliated with Indy and uses the web app's internal API, which can change without notice.

```sh
go build -o indy . && mv indy /usr/local/bin/
indy login                 # opens Chrome: log in normally, then press Enter in the terminal
indy whoami
indy tx list --from 2026-01-01 --has-receipts no --direction expense --all
indy tx categorize TX_ID SUB_ID 613600
indy tx vat TX_ID SUB_ID --rate 2000
indy receipt upload ./facture.pdf --tx TX_ID
indy help                  # every command
```

- Output is JSON on stdout. Errors go to stderr with exit code 1.
- Amounts are in cents. VAT rates are in basis points (2000 = 20%).
- The session is stored in `~/Library/Application Support/indy/session.json` (macOS) or `~/.config/indy/session.json` (Linux), mode 0600. It lasts about a year.
- `indy login` needs Google Chrome. It uses its own profile in the same folder as the session, so your normal Chrome is untouched. Without Chrome, copy the `__Http-sid` cookie from DevTools on app.indy.fr and run `indy login --cookie <value>`.
- `indy api METHOD /path [json]` calls any endpoint the CLI does not wrap.

Add `--toon` to any command to get [TOON](https://toonformat.dev/) instead of JSON (fewer tokens for agents).
