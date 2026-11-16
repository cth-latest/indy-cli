package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	toon "github.com/toon-format/toon-go"
)

var useToon bool

const usage = `indy - unofficial CLI for Indy (app.indy.fr). Output is JSON on stdout.
Add --toon to any command for TOON output (toonformat.dev): same data, fewer tokens for agents.

Auth:
  login                       log in through a Chrome window, then press Enter here
  login --cookie SID          use a __Http-sid cookie copied from your browser
  logout | whoami
  env: INDY_SESSION (sid cookie), INDY_ORG_ID, INDY_SESSION_FILE,
       INDY_NO_UPDATE_CHECK=1 (skip the check for a newer release, done at most every 15 min)
  upgrade [--check]           install the latest release (--check only reports it) | version

Transactions (amounts in cents, VAT rates in basis points: 2000 = 20%):
  tx list [filters] [--page N | --all]
  tx summary [filters]
  tx get ID | tx history ID
  tx categorize ID SUBDIVISION_ID ACCOUNT_NUMBER [--apply-receipt-vat]
  tx vat ID SUBDIVISION_ID (--rate 2000 | --type eu|not-eu|autoliquidation-fr|toBeDefined [--rate N])
  tx verify ID [--undo]
  tx annotate ID TEXT
  tx date ID YYYY-MM-DD
  tx delete ID | tx restore ID
  tx receipts ID | tx matching-receipts ID... | tx matching-invoices ID...
  tx to-categorize | tx categories | tx filters | tx pending
  filters: --from --to YYYY-MM-DD, --search TEXT, --account N[,N], --has-receipts yes|no,
           --verified yes|no, --vat-type fr,eu,not-eu,toBeDefined, --vat-rate 2000[,550],
           --min-cents N, --max-cents N, --direction income|expense, --bank ID[,ID],
           --type T[,T], --deleted both|yes

Receipts:
  receipt upload FILE [--tx ID]      upload (and attach to a transaction)
  receipt get ID | receipt delete ID | receipt unpaired
  receipt attach RECEIPT_ID TX_ID [--apply-data]
  receipt detach RECEIPT_ID
  receipt matching RECEIPT_ID        candidate transactions

Invoices:
  invoice list [--limit N] | invoice pdf ID [-o FILE] | invoice link INVOICE_ID TX_ID

VAT:
  vat config [--date YYYY-MM-DD] | vat declarations [--year Y] | vat history | vat next
  vat set-next YYYY-MM-DD | vat advance | vat periods
  vat lines --tag TAG --field ID | vat tx-details --tag TAG --tx ID
  vat export TAG                     export status of a declaration

Accounting documents (YEAR = fiscal year):
  doc periods | doc bilan YEAR | doc pnl YEAR | doc balance YEAR
  doc fec YEAR | doc journal YEAR | doc export NAME [--year Y] [--type pdf|xlsx|csv] [--tag TAG]

Other:
  bank accounts | bank connections | activities | dashboard [PERIOD_ID]
  closing DECLARATION                CA12|VAT_ADVANCE_PAYMENT|BICISFISCAL|BIC_IS_SOLDE_IS
  api METHOD PATH [JSON_BODY|@file]  raw call, PATH relative to /api (e.g. /transactions/to-categorize-count)
`

func main() {
	// --toon is global, so it is taken out before the per-command flag parsing.
	if i := slices.Index(os.Args, "--toon"); i > 0 {
		useToon, os.Args = true, slices.Delete(os.Args, i, i+1)
	}
	if len(os.Args) < 2 || os.Args[1] == "-h" || os.Args[1] == "--help" || os.Args[1] == "help" {
		fmt.Print(usage)
		return
	}
	err := run(os.Args[1], os.Args[2:])
	if os.Args[1] != "upgrade" {
		// On stderr, so the JSON on stdout stays clean.
		if n := updateNotice(func() (string, error) { return latestTag(2 * time.Second) }); n != "" {
			fmt.Fprintln(os.Stderr, n)
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.Status == 401 && os.Args[1] != "login" {
			fmt.Fprintln(os.Stderr, "session expired or invalid: run `indy login`")
		}
		os.Exit(1)
	}
}

func run(cmd string, args []string) error {
	switch cmd {
	case "login":
		return cmdLogin(args)
	case "logout":
		return cmdLogout(args)
	case "upgrade":
		return cmdUpgrade(args)
	case "version":
		fmt.Println(version)
		return nil
	}
	s, err := loadSession()
	if err != nil {
		return err
	}
	c := newClient(s)
	handlers := map[string]func(*Client, []string) (any, error){
		"whoami": cmdWhoami, "tx": cmdTx, "receipt": cmdReceipt, "invoice": cmdInvoice,
		"vat": cmdVat, "doc": cmdDoc, "bank": cmdBank, "api": cmdAPI,
		"activities": get("/activities"),
		"closing": func(c *Client, a []string) (any, error) {
			if err := need(a, 1, "closing DECLARATION"); err != nil {
				return nil, err
			}
			return c.JSON("GET", "/closing/all/refresh", url.Values{"declaration": {a[0]}, "fiscalPeriodId": {"current"}}, nil)
		},
		"dashboard": func(c *Client, a []string) (any, error) {
			id := "default"
			if len(a) > 0 {
				id = a[0]
			}
			return c.JSON("GET", "/dashboard/periods/"+url.PathEscape(id), nil, nil)
		},
	}
	h, ok := handlers[cmd]
	if !ok {
		return fmt.Errorf("unknown command %q (see `indy help`)", cmd)
	}
	out, err := h(c, args)
	if err != nil || out == nil {
		return err
	}
	if useToon {
		b, err := toon.Marshal(out)
		if err != nil {
			return err
		}
		_, err = fmt.Println(string(b))
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// parse parses flags anywhere in args (Go's flag stops at the first positional) and returns the positionals.
func parse(fs *flag.FlagSet, args []string) []string {
	var pos []string
	for {
		fs.Parse(args)
		args = fs.Args()
		if len(args) == 0 {
			return pos
		}
		pos, args = append(pos, args[0]), args[1:]
	}
}

func need(args []string, n int, use string) error {
	if len(args) < n {
		return fmt.Errorf("usage: indy %s", use)
	}
	return nil
}

func get(path string) func(*Client, []string) (any, error) {
	return func(c *Client, _ []string) (any, error) { return c.JSON("GET", path, nil, nil) }
}

func esc(s string) string { return url.PathEscape(s) }

func split(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}

type txFilter struct {
	from, to, search, accounts, hasReceipts, verified, vatTypes, vatRates, direction, banks, types, deleted string
	minCents, maxCents                                                                                      int64
}

func (f *txFilter) register(fs *flag.FlagSet) {
	fs.StringVar(&f.from, "from", "", "start date YYYY-MM-DD")
	fs.StringVar(&f.to, "to", "", "end date YYYY-MM-DD")
	fs.StringVar(&f.search, "search", "", "full-text search")
	fs.StringVar(&f.accounts, "account", "", "accounting account numbers, comma separated")
	fs.StringVar(&f.hasReceipts, "has-receipts", "", "yes|no")
	fs.StringVar(&f.verified, "verified", "", "yes|no")
	fs.StringVar(&f.vatTypes, "vat-type", "", "fr,eu,not-eu,toBeDefined")
	fs.StringVar(&f.vatRates, "vat-rate", "", "rates in basis points, comma separated")
	fs.Int64Var(&f.minCents, "min-cents", -1, "amount above (cents)")
	fs.Int64Var(&f.maxCents, "max-cents", -1, "amount under (cents)")
	fs.StringVar(&f.direction, "direction", "", "income|expense")
	fs.StringVar(&f.banks, "bank", "", "bank account ids, comma separated")
	fs.StringVar(&f.types, "type", "", "transaction types, comma separated")
	fs.StringVar(&f.deleted, "deleted", "", "both|yes (default: only non-deleted)")
}

// uiFilterDesc mirrors the web app's filter builder: empty values are omitted.
func (f *txFilter) build() (map[string]any, error) {
	m := map[string]any{}
	period := map[string]any{}
	if f.from != "" {
		period["startDate"] = f.from
	}
	if f.to != "" {
		period["endDate"] = f.to
	}
	if len(period) > 0 {
		m["period"] = period
	}
	if v := split(f.accounts); v != nil {
		m["accountingAccountNumber"] = v
	}
	if f.hasReceipts != "" {
		m["hasReceipts"] = f.hasReceipts
	}
	if v := split(f.vatTypes); v != nil {
		var out []string
		for _, t := range v {
			out = append(out, t)
			if t == "fr" { // the web app always pairs fr with French reverse charge
				out = append(out, "autoliquidation-fr")
			}
		}
		m["vatType"] = out
	}
	if v := split(f.vatRates); v != nil {
		var rates []int
		for _, r := range v {
			n, err := strconv.Atoi(r)
			if err != nil {
				return nil, fmt.Errorf("--vat-rate: %q is not an integer (basis points, 2000 = 20%%)", r)
			}
			rates = append(rates, n)
		}
		m["vatRate"] = rates
	}
	amt := map[string]any{}
	if f.minCents >= 0 {
		amt["above"] = f.minCents
	}
	if f.maxCents >= 0 {
		amt["under"] = f.maxCents
	}
	if f.direction != "" {
		amt["displayIncomeExpense"] = f.direction
	}
	if len(amt) > 0 {
		m["amountInCentsRange"] = amt
	}
	if f.search != "" {
		m["fullText"] = f.search
	}
	if v := split(f.banks); v != nil {
		m["bankAccountIds"] = v
	}
	if v := split(f.types); v != nil {
		m["transactionTypes"] = v
	}
	if f.deleted != "" {
		m["transactionIsDeleted"] = f.deleted
	}
	if f.verified != "" {
		m["transactionIsVerified"] = f.verified
	}
	return m, nil
}

func cmdTx(c *Client, args []string) (any, error) {
	if err := need(args, 1, "tx <subcommand> (see `indy help`)"); err != nil {
		return nil, err
	}
	sub, args := args[0], args[1:]
	fs := flag.NewFlagSet("tx "+sub, flag.ExitOnError)
	var f txFilter
	if sub == "list" || sub == "summary" {
		f.register(fs)
	}
	page := fs.Int("page", 1, "page index (30 per page)")
	all := fs.Bool("all", false, "fetch every page")
	applyVat := fs.Bool("apply-receipt-vat", false, "apply the receipt VAT")
	rate, vatType := new(int), new(string)
	if sub == "vat" { // --type means transaction types for list/summary
		rate = fs.Int("rate", -1, "VAT rate in basis points")
		vatType = fs.String("type", "fr", "VAT type")
	}
	undo := fs.Bool("undo", false, "mark as not verified")
	a := parse(fs, args)

	switch sub {
	case "list":
		filter, err := f.build()
		if err != nil {
			return nil, err
		}
		var acc []any
		var total any
		for p := *page; ; p++ {
			r, err := c.JSON("POST", "/transactions", nil, map[string]any{"uiFilterDesc": filter, "uiPaginationDesc": map[string]int{"pageIndex": p}})
			if err != nil {
				return nil, err
			}
			txs, _ := dig(r, "transactions").([]any)
			if !*all {
				return r, nil
			}
			acc, total = append(acc, txs...), dig(r, "totalMatchingTransactionsCount")
			if len(txs) == 0 {
				break
			}
			if n, ok := total.(float64); ok && len(acc) >= int(n) {
				break
			}
		}
		return map[string]any{"transactions": acc, "totalMatchingTransactionsCount": total}, nil
	case "summary":
		filter, err := f.build()
		if err != nil {
			return nil, err
		}
		return c.JSON("POST", "/accounting/transactions/summary", nil, map[string]any{"uiFilterDesc": filter})
	case "get":
		if err := need(a, 1, "tx get ID"); err != nil {
			return nil, err
		}
		return c.JSON("GET", "/accounting/transactions/"+esc(a[0]), nil, nil)
	case "history":
		if err := need(a, 1, "tx history ID"); err != nil {
			return nil, err
		}
		return c.JSON("GET", "/transactions/"+esc(a[0])+"/history", nil, nil)
	case "categorize":
		if err := need(a, 3, "tx categorize ID SUBDIVISION_ID ACCOUNT_NUMBER"); err != nil {
			return nil, err
		}
		return c.JSON("PUT", "/accounting/transactions/"+esc(a[0])+"/subdivisions/"+esc(a[1])+"/accounting-account-number", nil,
			map[string]any{"newAccountingAccountNumber": a[2], "applyReceiptVat": *applyVat})
	case "vat":
		if err := need(a, 2, "tx vat ID SUBDIVISION_ID --rate N [--type T]"); err != nil {
			return nil, err
		}
		vat, err := newVat(*vatType, *rate)
		if err != nil {
			return nil, err
		}
		return c.JSON("PUT", "/accounting/transactions/"+esc(a[0])+"/subdivisions/"+esc(a[1])+"/vat", nil, map[string]any{"newVat": vat})
	case "verify":
		if err := need(a, 1, "tx verify ID [--undo]"); err != nil {
			return nil, err
		}
		return c.JSON("POST", "/transactions/"+esc(a[0])+"/verification", nil, map[string]bool{"isVerified": !*undo})
	case "annotate":
		if err := need(a, 2, "tx annotate ID TEXT"); err != nil {
			return nil, err
		}
		return c.JSON("POST", "/transactions/"+esc(a[0])+"/annotation", nil, map[string]string{"annotation": strings.Join(a[1:], " ")})
	case "date":
		if err := need(a, 2, "tx date ID YYYY-MM-DD"); err != nil {
			return nil, err
		}
		return c.JSON("POST", "/transactions/"+esc(a[0])+"/date", nil, map[string]string{"newDate": a[1]})
	case "delete":
		if err := need(a, 1, "tx delete ID"); err != nil {
			return nil, err
		}
		return c.JSON("DELETE", "/transactions/"+esc(a[0]), nil, nil)
	case "restore":
		if err := need(a, 1, "tx restore ID"); err != nil {
			return nil, err
		}
		return c.JSON("POST", "/transactions/"+esc(a[0])+"/resurrect", nil, nil)
	case "receipts":
		if err := need(a, 1, "tx receipts ID"); err != nil {
			return nil, err
		}
		return c.JSON("GET", "/receipts/transaction", url.Values{"transactionId": {a[0]}}, nil)
	case "matching-receipts", "matching-invoices":
		if err := need(a, 1, "tx "+sub+" ID..."); err != nil {
			return nil, err
		}
		return c.JSON("POST", "/transactions/get-"+sub, nil, map[string]any{"transactionIds": a})
	case "to-categorize":
		return c.JSON("GET", "/transactions/to-categorize-count", nil, nil)
	case "categories":
		return c.JSON("GET", "/transactions/category-groups-with-decorations", nil, nil)
	case "filters":
		return c.JSON("GET", "/transactions/transactions-list-filters", nil, nil)
	case "pending":
		return c.JSON("GET", "/transactions/transactions-pending-list", nil, nil)
	}
	return nil, fmt.Errorf("unknown subcommand tx %s", sub)
}

// newVat mirrors the web app's VAT payload: {type, rate}.
func newVat(typ string, rate int) (map[string]any, error) {
	switch typ {
	case "toBeDefined":
		return map[string]any{"type": typ, "rate": 0}, nil
	case "fr", "eu", "not-eu", "autoliquidation-fr":
		if rate < 0 {
			return nil, errors.New("--rate is required (basis points, e.g. 2000 = 20%, 0 for exempt)")
		}
		return map[string]any{"type": typ, "rate": rate}, nil
	}
	return nil, fmt.Errorf("unknown VAT type %q (fr, eu, not-eu, autoliquidation-fr, toBeDefined)", typ)
}

func cmdReceipt(c *Client, args []string) (any, error) {
	if err := need(args, 1, "receipt <subcommand>"); err != nil {
		return nil, err
	}
	sub, args := args[0], args[1:]
	fs := flag.NewFlagSet("receipt "+sub, flag.ExitOnError)
	tx := fs.String("tx", "", "transaction id to attach to")
	applyData := fs.Bool("apply-data", false, "apply receipt data (amount, VAT) to the transaction")
	a := parse(fs, args)
	switch sub {
	case "upload":
		if err := need(a, 1, "receipt upload FILE [--tx ID]"); err != nil {
			return nil, err
		}
		var q url.Values
		if *tx != "" {
			q = url.Values{"transactionId": {*tx}}
		}
		return c.Upload("/receipts", q, a[0])
	case "unpaired":
		return c.JSON("GET", "/receipts/unpaired", nil, nil)
	}
	if err := need(a, 1, "receipt "+sub+" RECEIPT_ID"); err != nil {
		return nil, err
	}
	id := esc(a[0])
	switch sub {
	case "get":
		return c.JSON("GET", "/receipts/"+id, nil, nil)
	case "delete":
		return c.JSON("DELETE", "/receipts/"+id, nil, nil)
	case "attach":
		if err := need(a, 2, "receipt attach RECEIPT_ID TX_ID"); err != nil {
			return nil, err
		}
		return c.JSON("PUT", "/receipts/"+id+"/transaction", nil, map[string]any{"transactionId": a[1], "applyReceiptData": *applyData})
	case "detach":
		return c.JSON("POST", "/receipts/"+id+"/detach", nil, nil)
	case "matching":
		return c.JSON("GET", "/receipts/v2/"+id+"/matching-transactions", nil, nil)
	}
	return nil, fmt.Errorf("unknown subcommand receipt %s", sub)
}

func cmdInvoice(c *Client, args []string) (any, error) {
	if err := need(args, 1, "invoice <subcommand>"); err != nil {
		return nil, err
	}
	sub, args := args[0], args[1:]
	fs := flag.NewFlagSet("invoice "+sub, flag.ExitOnError)
	limit := fs.Int("limit", 50, "max invoices")
	out := fs.String("o", "", "output file (default ID.pdf, - for stdout)")
	a := parse(fs, args)
	switch sub {
	case "list":
		return c.JSON("GET", "/invoices", url.Values{"type": {"indy"}, "limit": {strconv.Itoa(*limit)}}, nil)
	case "pdf":
		if err := need(a, 1, "invoice pdf ID [-o FILE]"); err != nil {
			return nil, err
		}
		if *out == "" {
			*out = a[0] + ".pdf"
		}
		return nil, c.Download("/facturation/invoices/"+esc(a[0])+"/documents/invoice", nil, *out)
	case "link":
		if err := need(a, 2, "invoice link INVOICE_ID TX_ID"); err != nil {
			return nil, err
		}
		return c.JSON("PUT", "/facturation-invoices/"+esc(a[0])+"/transaction", nil, map[string]string{"transactionId": a[1]})
	}
	return nil, fmt.Errorf("unknown subcommand invoice %s", sub)
}

func cmdVat(c *Client, args []string) (any, error) {
	if err := need(args, 1, "vat <subcommand>"); err != nil {
		return nil, err
	}
	sub, args := args[0], args[1:]
	fs := flag.NewFlagSet("vat "+sub, flag.ExitOnError)
	date := fs.String("date", "", "date YYYY-MM-DD (default today)")
	year := fs.String("year", "", "fiscal year")
	tag := fs.String("tag", "", "declaration tag (from `vat declarations`)")
	field := fs.String("field", "", "declaration field id")
	tx := fs.String("tx", "", "transaction id")
	a := parse(fs, args)
	switch sub {
	case "config":
		q := url.Values{}
		if *date != "" {
			q.Set("date", *date)
		}
		return c.JSON("GET", "/vat/configuration", q, nil)
	case "declarations":
		q := url.Values{}
		if *year != "" {
			q.Set("fiscalYear", *year)
		}
		return c.JSON("GET", "/documents/vat/declarations", q, nil)
	case "history":
		return c.JSON("GET", "/vat/declarations", nil, nil)
	case "next":
		return c.JSON("GET", "/vat/next-declaration", nil, nil)
	case "set-next":
		if err := need(a, 1, "vat set-next YYYY-MM-DD"); err != nil {
			return nil, err
		}
		return c.JSON("PUT", "/vat/next-declaration", nil, map[string]string{"nextVatDeclarationDate": a[0]})
	case "advance":
		return c.JSON("GET", "/activities/vatAdvancePayment/current", nil, nil)
	case "periods":
		return c.JSON("GET", "/documents/vat/period-choices", nil, nil)
	case "lines":
		return c.JSON("GET", "/documents/vat/line-details", url.Values{"tvaTag": {*tag}, "fieldId": {*field}}, nil)
	case "tx-details":
		return c.JSON("GET", "/documents/vat/transaction-details", url.Values{"tvaTag": {*tag}, "transactionId": {*tx}}, nil)
	case "export":
		if err := need(a, 1, "vat export TAG"); err != nil {
			return nil, err
		}
		return c.JSON("GET", "/exports/documents/vat/"+esc(a[0])+"/export-status", nil, nil)
	}
	return nil, fmt.Errorf("unknown subcommand vat %s", sub)
}

func cmdDoc(c *Client, args []string) (any, error) {
	if err := need(args, 1, "doc <subcommand>"); err != nil {
		return nil, err
	}
	sub, args := args[0], args[1:]
	fs := flag.NewFlagSet("doc "+sub, flag.ExitOnError)
	year := fs.String("year", "", "fiscal year")
	typ := fs.String("type", "pdf", "pdf|xlsx|csv")
	tag := fs.String("tag", "", "VAT declaration tag")
	a := parse(fs, args)
	paths := map[string]string{"bilan": "/documents/bilan/", "pnl": "/documents/compte-de-resultat/",
		"balance": "/documents/general-balance/", "fec": "/exports/fec/", "journal": "/exports/accounting-journal/"}
	switch sub {
	case "periods":
		return c.JSON("GET", "/documents/period-choices", nil, nil)
	case "export":
		if err := need(a, 1, "doc export NAME [--year Y] [--type pdf]"); err != nil {
			return nil, err
		}
		q := url.Values{"type": {*typ}}
		if *year != "" {
			q.Set("year", *year)
		}
		if *tag != "" {
			q.Set("tvaTag", *tag)
		}
		return c.JSON("GET", "/exports/document/"+esc(a[0]), q, nil)
	}
	if p, ok := paths[sub]; ok {
		if err := need(a, 1, "doc "+sub+" YEAR"); err != nil {
			return nil, err
		}
		return c.JSON("GET", p+esc(a[0]), nil, nil)
	}
	return nil, fmt.Errorf("unknown subcommand doc %s", sub)
}

func cmdBank(c *Client, args []string) (any, error) {
	if err := need(args, 1, "bank accounts|connections"); err != nil {
		return nil, err
	}
	switch args[0] {
	case "accounts":
		return c.JSON("GET", "/bank-connector/bank-accounts", url.Values{"withAvailableBalanceInCents": {"true"}, "withConnectorBankAccountStatus": {"true"}}, nil)
	case "connections":
		return c.JSON("GET", "/bank-connector/bank-auths", url.Values{"withBankAccounts": {"true"}}, nil)
	}
	return nil, fmt.Errorf("unknown subcommand bank %s", args[0])
}

func cmdAPI(c *Client, args []string) (any, error) {
	if err := need(args, 2, "api METHOD PATH [JSON_BODY|@file]"); err != nil {
		return nil, err
	}
	u, err := url.Parse(strings.TrimPrefix(args[1], "/api"))
	if err != nil {
		return nil, err
	}
	var body any
	if len(args) > 2 {
		raw := []byte(args[2])
		if strings.HasPrefix(args[2], "@") {
			if raw, err = os.ReadFile(args[2][1:]); err != nil {
				return nil, err
			}
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			return nil, fmt.Errorf("body is not valid JSON: %w", err)
		}
	}
	return c.JSON(strings.ToUpper(args[0]), u.Path, u.Query(), body)
}
