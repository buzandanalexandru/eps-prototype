// Package tickets issues tickets from confirmed payments and serves their rotating entry codes
// (step 7; O5, P3): the 104-byte Ed25519-signed token, the server's signing keys and the public key
// set, issuing and reissuing (generations and the door feed), suspension from instalment plans,
// personalisation, online check-in, day picking, the holder's trip and the GDPR erasure.
package tickets

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Token layout (design D2): version, key id, event, ticket, generation, window, signature.
const (
	TokenVersion = 1
	bodyLen      = 1 + 1 + 16 + 16 + 2 + 4
	TokenLen     = bodyLen + ed25519.SignatureSize // 104
	// CodePrefix starts every entry code (the QR's text).
	CodePrefix = "E1."
	// Window is one code's time window.
	Window = 15 * time.Second
	// BufferWindows is how many windows the holder gets at once: the current one and 39 more, 10
	// minutes (D4), the longest a revoked holder can still show codes.
	BufferWindows = 40
)

// Token is a decoded entry code.
type Token struct {
	KeyID      byte
	EventID    string
	TicketID   string
	Generation uint16
	Window     uint32
}

// WindowOf is the code window of a time: floor(unix seconds / 15).
func WindowOf(t time.Time) uint32 { return uint32(t.Unix() / int64(Window/time.Second)) }

// WindowStart is when window w begins.
func WindowStart(w uint32) time.Time { return time.Unix(int64(w)*int64(Window/time.Second), 0).UTC() }

var (
	ErrMalformed  = errors.New("tickets: not a valid code")
	ErrUnknownKey = errors.New("tickets: unknown key id")
	ErrSignature  = errors.New("tickets: bad signature")
)

func (t Token) body() ([]byte, error) {
	ev, err := uuid.Parse(t.EventID)
	if err != nil {
		return nil, ErrMalformed
	}
	tk, err := uuid.Parse(t.TicketID)
	if err != nil {
		return nil, ErrMalformed
	}
	b := make([]byte, bodyLen, TokenLen)
	b[0] = TokenVersion
	b[1] = t.KeyID
	copy(b[2:18], ev[:])
	copy(b[18:34], tk[:])
	binary.BigEndian.PutUint16(b[34:36], t.Generation)
	binary.BigEndian.PutUint32(b[36:40], t.Window)
	return b, nil
}

// Sign makes the entry code of a token with the private key of its key id.
func Sign(t Token, priv ed25519.PrivateKey) (string, error) {
	b, err := t.body()
	if err != nil {
		return "", err
	}
	b = append(b, ed25519.Sign(priv, b)...)
	return CodePrefix + base64.RawURLEncoding.EncodeToString(b), nil
}

// Decode parses a code without checking its signature.
func Decode(code string) (Token, []byte, error) {
	if !strings.HasPrefix(code, CodePrefix) || len(code) > 200 {
		return Token{}, nil, ErrMalformed
	}
	b, err := base64.RawURLEncoding.DecodeString(code[len(CodePrefix):])
	if err != nil || len(b) != TokenLen || b[0] != TokenVersion {
		return Token{}, nil, ErrMalformed
	}
	ev, _ := uuid.FromBytes(b[2:18])
	tk, _ := uuid.FromBytes(b[18:34])
	return Token{KeyID: b[1], EventID: ev.String(), TicketID: tk.String(), Generation: binary.BigEndian.Uint16(b[34:36]), Window: binary.BigEndian.Uint32(b[36:40])}, b, nil
}

// Verify decodes a code and checks its signature with the public key of its key id.
func Verify(code string, keys map[byte]ed25519.PublicKey) (Token, error) {
	t, b, err := Decode(code)
	if err != nil {
		return t, err
	}
	pub, ok := keys[t.KeyID]
	if !ok {
		return t, ErrUnknownKey
	}
	if !ed25519.Verify(pub, b[:bodyLen], b[bodyLen:]) {
		return t, ErrSignature
	}
	return t, nil
}
