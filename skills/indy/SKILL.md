---
name: indy
description: Use the `indy` CLI for Indy (app.indy.fr) accounting - transactions, categories, VAT, receipts.
---

# Indy CLI

`indy` is an unofficial command-line client for Indy. Every command prints JSON on stdout; errors go to stderr with exit code 1. `indy help` lists every command and flag: read it before guessing a command.

## Setup

1. Check the CLI: `indy version`. If it is missing, install it:
   `curl -fsSL https://raw.githubusercontent.com/cth-latest/indy-cli/main/install.sh | sh`
   (installs to `~/.local/bin`; `indy upgrade` updates it).
2. Check the session: `indy whoami`.
3. If it says "not logged in" or "session expired", **ask the user to run `indy login` themselves**. Login opens a Chrome window and needs a human (captcha, password, MFA) who then presses Enter in the terminal. Do not run it for them and never ask for their password.

## Rules

- **Units:** amounts are in cents (`-18000` = an expense of 180.00 EUR). VAT rates are in basis points (`2000` = 20%, `550` = 5.5%).
- **Save tokens:** add `--toon` to any read command for a compact format. Use plain JSON when piping to `jq`.
- **Narrow before listing:** `tx list` returns 30 per page. Use filters, and `indy tx summary [filters]` for counts and totals, before `--all`.
- **Confirm before destructive or bulk changes:** ask the user before `tx delete`, `receipt delete`, `receipt detach`, `tx verify`, or changing more than a few transactions in one go. Say what you will change first.
- **Do not invent accounting choices.** If the right category or VAT rate is unclear, show the user the candidates and ask.
- The API is Indy's internal one and can change: if a command fails with an unexpected HTTP error, report it instead of retrying variants.

## Transactions

```sh
indy tx list --from 2026-01-01 --to 2026-03-31 --direction expense --has-receipts no
indy tx list --search "OVH" --all
indy tx get TX_ID
```

Filters: `--from --to`, `--search`, `--account N[,N]`, `--has-receipts yes|no`, `--verified yes|no`, `--vat-type fr,eu,not-eu,toBeDefined`, `--vat-rate 2000[,550]`, `--min-cents`, `--max-cents`, `--direction income|expense`, `--bank ID[,ID]`.

Each transaction has `subdivisions`. Categorizing and VAT apply to one subdivision, so you need its `id`:
the bank-side subdivision has `"modifiable": false` (account `512...`); **use the other one**.

### Categorize

```sh
indy tx to-categorize                 # how many are left
indy tx categories                    # category groups, with the account numbers
indy tx categorize TX_ID SUBDIVISION_ID 626000
```

`ACCOUNT_NUMBER` is the accounting account number of the chosen category, taken from `tx categories`. Add `--apply-receipt-vat` to reuse the VAT read from an attached receipt.

### VAT

```sh
indy tx vat TX_ID SUBDIVISION_ID --rate 2000                    # French VAT at 20%
indy tx vat TX_ID SUBDIVISION_ID --type eu --rate 2000          # eu | not-eu | autoliquidation-fr
indy tx vat TX_ID SUBDIVISION_ID --type toBeDefined             # clear it
```

### Other

```sh
indy tx annotate TX_ID "note"
indy tx verify TX_ID          # --undo to revert
indy tx history TX_ID
```

## Receipts

```sh
indy receipt upload ./invoice.pdf --tx TX_ID     # upload and attach
indy receipt upload ./invoice.pdf                # upload only
indy receipt unpaired                            # receipts with no transaction
indy receipt matching RECEIPT_ID                 # candidate transactions
indy receipt attach RECEIPT_ID TX_ID --apply-data
indy tx receipts TX_ID
```

To attach a batch of files: for each file, find the transaction with `tx list --search` or amount filters (`--min-cents`/`--max-cents`) and dates, and only attach when exactly one transaction matches; list the ambiguous ones for the user.

## VAT declarations and documents

```sh
indy vat config
indy vat declarations --year 2026
indy vat next
indy doc periods
indy doc bilan 2026
indy doc pnl 2026
indy doc balance 2026
indy doc fec 2026
```

## Anything else

`indy bank accounts`, `indy activities`, `indy dashboard`, and `indy api METHOD /path [json]` for endpoints the CLI does not wrap. Treat `indy api` with a non-GET method as a change that needs the user's confirmation.
