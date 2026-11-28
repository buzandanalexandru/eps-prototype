package tickets_test

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"os"
	"testing"

	"github.com/buzandanalexandru/eps-prototype/services/internal/tickets"
)

var update = flag.Bool("update", false, "rewrite testdata/vectors.json")

// Vectors shared with the web verifier (apps/web/tests/unit/ticket-token.test.ts).
type vectors struct {
	SeedHex   string   `json:"seedHex"`
	PublicKey string   `json:"publicKey"`
	KeyID     int      `json:"keyId"`
	Valid     []vector `json:"valid"`
	Invalid   []vector `json:"invalid"`
}

type vector struct {
	Name       string `json:"name"`
	Code       string `json:"code"`
	EventID    string `json:"eventId,omitempty"`
	TicketID   string `json:"ticketId,omitempty"`
	Generation int    `json:"generation,omitempty"`
	Window     uint32 `json:"window,omitempty"`
	Error      string `json:"error,omitempty"`
}

const (
	vecEvent  = "1b4e28ba-2fa1-4d2a-883f-0016d3cca427"
	vecTicket = "6f1c1a3e-9b7a-4c55-b1b0-3d1f2e4a5b6c"
)

func vecKey() ed25519.PrivateKey {
	seed, _ := hex.DecodeString("9d61b19deffd5a60ba844af492ec2cc44449c5697b326919703bac031cae7f60")
	return ed25519.NewKeyFromSeed(seed)
}

func alter(code string, byteIndex int) string {
	b, _ := base64.RawURLEncoding.DecodeString(code[3:])
	b[byteIndex] ^= 0x01
	return "E1." + base64.RawURLEncoding.EncodeToString(b)
}

func buildVectors(t *testing.T) vectors {
	priv := vecKey()
	v := vectors{SeedHex: "9d61b19deffd5a60ba844af492ec2cc44449c5697b326919703bac031cae7f60", KeyID: 1,
		PublicKey: base64.RawURLEncoding.EncodeToString(priv.Public().(ed25519.PublicKey))}
	for _, x := range []struct {
		gen uint16
		w   uint32
	}{{1, 118_000_000}, {2, 118_000_001}, {65535, 4_000_000_000}} {
		c, err := tickets.Sign(tickets.Token{KeyID: 1, EventID: vecEvent, TicketID: vecTicket, Generation: x.gen, Window: x.w}, priv)
		if err != nil {
			t.Fatal(err)
		}
		v.Valid = append(v.Valid, vector{Name: "valid", Code: c, EventID: vecEvent, TicketID: vecTicket, Generation: int(x.gen), Window: x.w})
	}
	base := v.Valid[0].Code
	other, _ := tickets.Sign(tickets.Token{KeyID: 7, EventID: vecEvent, TicketID: vecTicket, Generation: 1, Window: 118_000_000}, priv)
	v.Invalid = []vector{
		{Name: "generation changed", Code: alter(base, 35), Error: "signature"},
		{Name: "window changed", Code: alter(base, 39), Error: "signature"},
		{Name: "ticket changed", Code: alter(base, 20), Error: "signature"},
		{Name: "signature changed", Code: alter(base, 70), Error: "signature"},
		{Name: "unknown key id", Code: other, Error: "unknown_key"},
		{Name: "truncated", Code: base[:len(base)-4], Error: "malformed"},
		{Name: "wrong prefix", Code: "E2." + base[3:], Error: "malformed"},
	}
	return v
}

// Scenarios "Code verifies with the published public key" and "Altered code fails", and the fixed
// vectors the browser verifier must agree with ("Same test vectors in Go and the browser").
func TestTokenVectors(t *testing.T) {
	want := buildVectors(t)
	if *update {
		b, _ := json.MarshalIndent(want, "", "  ")
		if err := os.WriteFile("testdata/vectors.json", append(b, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := os.ReadFile("testdata/vectors.json")
	if err != nil {
		t.Fatalf("read vectors (run with -update to create them): %v", err)
	}
	var got vectors
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	pub, _ := base64.RawURLEncoding.DecodeString(got.PublicKey)
	keys := map[byte]ed25519.PublicKey{1: ed25519.PublicKey(pub)}
	if len(got.Valid) != len(want.Valid) {
		t.Fatalf("vectors file is stale: run go test -run TestTokenVectors -update")
	}
	for i, v := range got.Valid {
		if v.Code != want.Valid[i].Code {
			t.Fatalf("valid vector %d differs from what the signer makes now (Ed25519 is deterministic)", i)
		}
		if len(v.Code) > 145 {
			t.Fatalf("code is %d characters, want about 143 (a version 8 QR)", len(v.Code))
		}
		tok, err := tickets.Verify(v.Code, keys)
		if err != nil {
			t.Fatalf("valid vector %d: %v", i, err)
		}
		if tok.EventID != v.EventID || tok.TicketID != v.TicketID || int(tok.Generation) != v.Generation || tok.Window != v.Window {
			t.Fatalf("vector %d decoded to %+v", i, tok)
		}
	}
	for _, v := range got.Invalid {
		_, err := tickets.Verify(v.Code, keys)
		want := map[string]error{"signature": tickets.ErrSignature, "unknown_key": tickets.ErrUnknownKey, "malformed": tickets.ErrMalformed}[v.Error]
		if err != want {
			t.Fatalf("%s: got %v, want %v", v.Name, err, want)
		}
	}
}

func TestTokenHasNoPersonalData(t *testing.T) {
	c, _ := tickets.Sign(tickets.Token{KeyID: 1, EventID: vecEvent, TicketID: vecTicket, Generation: 1, Window: 5}, vecKey())
	_, b, err := tickets.Decode(c)
	if err != nil || len(b) != tickets.TokenLen {
		t.Fatalf("decode: %v, %d bytes", err, len(b))
	}
}
