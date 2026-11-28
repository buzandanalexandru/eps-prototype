package fakepsp

import (
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strings"
)

// The hosted page (D-6, D13): test outcomes for cards, simulated Apple Pay and Google Pay sheets
// and the simulated bank code. There is no card number field: nothing card-like is ever typed.

type pageText struct {
	Lang, Title, Notice, Amount, Approve, DeclineFunds, Decline3DS, Cancel, ApplePay, GooglePay, SheetNote string
	ChallengeTitle, ChallengeLead, BankCode, CodeLabel, Submit, TriesLeft, WrongCode                       string
	Done, Approved, Declined, Back, Simulated, Merchant, Order                                             string
}

var texts = map[string]pageText{
	"en": {Lang: "en", Title: "Simulated payment provider", Notice: "Test page: no real payment is made and no card data is used.", Amount: "Amount",
		Approve: "Test card: approve", DeclineFunds: "Decline: insufficient funds", Decline3DS: "Decline: 3-D Secure failed", Cancel: "Cancel at bank",
		ApplePay: "Pay with Apple Pay (simulated)", GooglePay: "Pay with Google Pay (simulated)", SheetNote: "A simulated wallet sheet: confirm to pay.",
		ChallengeTitle: "Confirm with your bank (simulated)", ChallengeLead: "Enter the code your bank sent you.", BankCode: "Simulated bank code", CodeLabel: "Code",
		Submit: "Confirm", TriesLeft: "Tries left", WrongCode: "Wrong code.", Done: "Payment finished", Approved: "Approved", Declined: "Declined",
		Back: "Back to the shop", Simulated: "simulated", Merchant: "Paying", Order: "order"},
	"ro": {Lang: "ro", Title: "Furnizor de plăți simulat", Notice: "Pagină de test: nu se face nicio plată reală și nu se folosesc date de card.", Amount: "Sumă",
		Approve: "Card de test: aprobă", DeclineFunds: "Refuz: fonduri insuficiente", Decline3DS: "Refuz: 3-D Secure eșuat", Cancel: "Anulează la bancă",
		ApplePay: "Plătește cu Apple Pay (simulat)", GooglePay: "Plătește cu Google Pay (simulat)", SheetNote: "Un ecran de portofel simulat: confirmă pentru a plăti.",
		ChallengeTitle: "Confirmă la bancă (simulat)", ChallengeLead: "Introdu codul trimis de bancă.", BankCode: "Cod bancar simulat", CodeLabel: "Cod",
		Submit: "Confirmă", TriesLeft: "Încercări rămase", WrongCode: "Cod greșit.", Done: "Plată încheiată", Approved: "Aprobată", Declined: "Refuzată",
		Back: "Înapoi la magazin", Simulated: "simulat", Merchant: "Plătești către", Order: "comanda"},
}

var pageTmpl = template.Must(template.New("page").Parse(`<!doctype html>
<html lang="{{.T.Lang}}"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="robots" content="noindex"><title>{{.T.Title}} {{.Provider}}</title>
<style>
body{font-family:system-ui,-apple-system,"Segoe UI",sans-serif;margin:0;background:#f2f4f7;color:#101828}
main{max-width:28rem;margin:0 auto;padding:1.5rem 1rem}
.card{background:#fff;border-radius:12px;padding:1.25rem;box-shadow:0 1px 3px #0002}
h1{font-size:1.25rem;margin:0 0 .5rem}.note{font-size:.875rem;background:#fff4d6;border-radius:8px;padding:.5rem .75rem}
.amount{font-size:1.75rem;font-weight:700;margin:1rem 0}
button{display:block;width:100%;margin:.5rem 0;padding:.8rem;border-radius:10px;border:1px solid #98a2b3;background:#fff;font-size:1rem;cursor:pointer}
button.primary{background:#101828;color:#fff;border-color:#101828}
label{display:block;margin-top:.75rem;font-weight:600}input{font-size:1.25rem;padding:.5rem;width:8rem;letter-spacing:.2em}
.code{font-family:ui-monospace,monospace;font-size:1.25rem;font-weight:700}
.err{color:#b42318}a{color:#101828}
</style></head><body><main><div class="card" data-testid="psp-page" data-status="{{.Status}}" data-method="{{.Method}}">
<h1>{{.T.Title}} {{.Provider}}</h1>
{{if .Merchant}}<p data-testid="psp-merchant">{{.T.Merchant}}: <strong>{{.Merchant}}</strong>{{if .OrderRef}} · {{.T.Order}} <span data-testid="psp-order">{{.OrderRef}}</span>{{end}}</p>{{end}}
<p class="note" role="note">{{.T.Notice}}</p>
<p>{{.T.Amount}}</p><p class="amount" data-testid="psp-amount">{{.Amount}}</p>
{{if eq .Status "pending"}}
<form method="post" action="{{.Action}}">
{{if eq .Method "apple_pay"}}<p data-testid="psp-sheet">{{.T.SheetNote}}</p><button class="primary" name="outcome" value="approve" data-testid="psp-wallet-pay">{{.T.ApplePay}}</button>
{{else if eq .Method "google_pay"}}<p data-testid="psp-sheet">{{.T.SheetNote}}</p><button class="primary" name="outcome" value="approve" data-testid="psp-wallet-pay">{{.T.GooglePay}}</button>
{{else}}<button class="primary" name="outcome" value="approve" data-testid="psp-approve">{{.T.Approve}}</button>
<button name="outcome" value="decline_funds" data-testid="psp-decline-funds">{{.T.DeclineFunds}}</button>
<button name="outcome" value="decline_3ds" data-testid="psp-decline-3ds">{{.T.Decline3DS}}</button>{{end}}
<button name="outcome" value="cancel" data-testid="psp-cancel">{{.T.Cancel}}</button>
</form>
{{else if eq .Status "requires_action"}}
<h2>{{.T.ChallengeTitle}}</h2><p>{{.T.ChallengeLead}}</p>
<p>{{.T.BankCode}}: <span class="code" data-testid="psp-bank-code">{{.Code}}</span></p>
{{if .Wrong}}<p class="err" role="alert" data-testid="psp-wrong-code">{{.T.WrongCode}} {{.T.TriesLeft}}: {{.Left}}</p>{{end}}
<form method="post" action="{{.Action}}/challenge"><label for="code">{{.T.CodeLabel}}</label>
<input id="code" name="code" inputmode="numeric" maxlength="6" autocomplete="one-time-code" pattern="[0-9]{6}" required data-testid="psp-code">
<button class="primary" data-testid="psp-submit-code">{{.T.Submit}}</button></form>
{{else}}
<p data-testid="psp-result">{{.T.Done}}: {{if eq .Status "succeeded"}}{{.T.Approved}}{{else}}{{.T.Declined}}{{end}}</p>
{{if .Return}}<p><a href="{{.Return}}" data-testid="psp-back">{{.T.Back}}</a></p>{{end}}
{{end}}
</div></main></body></html>`))

func money(minor int64, currency, locale string) string {
	major, cents := minor/100, minor%100
	if locale == "ro" {
		return fmt.Sprintf("%d,%02d %s", major, cents, currency)
	}
	return fmt.Sprintf("%s %d.%02d", currency, major, cents)
}

func origin(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

func (p *Provider) headers(w http.ResponseWriter, returnURL string) {
	fa := "'self'"
	if o := origin(returnURL); o != "" {
		// After a form post the page redirects back to the shop: form-action covers redirects.
		fa += " " + o
	}
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action "+fa+"; frame-ancestors 'none'; base-uri 'none'")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "no-store")
}

func (p *Provider) render(w http.ResponseWriter, r *http.Request, s *stored, wrong bool) {
	t := texts[s.Locale]
	if t.Lang == "" {
		t = texts["ro"]
	}
	p.headers(w, s.ReturnURL)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pageTmpl.Execute(w, map[string]any{
		"T": t, "Provider": strings.ToUpper(p.ID), "Status": s.Status, "Method": s.Method, "Amount": money(s.AmountMinor, s.Currency, t.Lang),
		"Action": "/" + p.ID + "/pay/" + s.ID, "Code": s.ChallengeCode, "Wrong": wrong, "Left": 3 - s.ChallengeTries, "Return": template.URL(s.ReturnURL),
		"Merchant": s.Metadata["merchant_name"], "OrderRef": s.Metadata["order_ref"],
	})
}

func (p *Provider) pageCharge(w http.ResponseWriter, r *http.Request) *stored {
	id := r.PathValue("id")
	if !idPattern.MatchString(id) {
		http.NotFound(w, r)
		return nil
	}
	s, err := p.loadCharge(r.Context(), id)
	if err != nil {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return nil
	}
	if s == nil || s.Method == SavedCard && s.Status != RequiresAction && s.Status != Succeeded && s.Status != Declined {
		http.NotFound(w, r)
		return nil
	}
	return s
}

func (p *Provider) page(w http.ResponseWriter, r *http.Request) {
	if s := p.pageCharge(w, r); s != nil {
		p.render(w, r, s, r.URL.Query().Get("wrong") == "1")
	}
}

// back sends the fan to the shop once the charge is final, else to the page again.
func (p *Provider) back(w http.ResponseWriter, r *http.Request, s *stored, wrong bool) {
	p.headers(w, s.ReturnURL)
	if (s.Status == Succeeded || s.Status == Declined) && s.ReturnURL != "" {
		http.Redirect(w, r, s.ReturnURL, http.StatusSeeOther)
		return
	}
	to := "/" + p.ID + "/pay/" + s.ID
	if wrong {
		to += "?wrong=1"
	}
	http.Redirect(w, r, to, http.StatusSeeOther)
}

func (p *Provider) pagePost(w http.ResponseWriter, r *http.Request) {
	s := p.pageCharge(w, r)
	if s == nil {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	s, err := p.Outcome(r.Context(), s.ID, r.PostForm.Get("outcome"))
	if err != nil || s == nil {
		http.Error(w, "bad outcome", http.StatusBadRequest)
		return
	}
	p.back(w, r, s, false)
}

func (p *Provider) pageChallenge(w http.ResponseWriter, r *http.Request) {
	s := p.pageCharge(w, r)
	if s == nil {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	before := s.ChallengeTries
	s, err := p.Challenge(r.Context(), s.ID, strings.TrimSpace(r.PostForm.Get("code")))
	if err != nil || s == nil {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	p.back(w, r, s, s.ChallengeTries > before && s.Status == RequiresAction)
}
