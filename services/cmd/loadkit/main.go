// Command loadkit prepares a local load test of the on-sale engine (step 5; load/inventory.js and
// load/queue.js): a published event with one big pool, optionally a sale moment with its waiting
// room, a token signing key the core trusts, and N short-lived guest core tokens, each with its own
// session secret and proof of work. Development only: it refuses to run unless EPS_ENV is dev and
// it writes straight into the database as the migrating owner. The simulator (step 6) replaces it
// for rehearsals on a deployment.
//
//	go run ./cmd/loadkit -n 30000 -capacity 20000 -out /tmp/load.json            (inventory)
//	go run ./cmd/loadkit -n 20000 -queue -rate 60000 -out /tmp/queue.json        (waiting room)
package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buzandanalexandru/eps-prototype/services/internal/auth"
	"github.com/buzandanalexandru/eps-prototype/services/internal/queue"
	"github.com/buzandanalexandru/eps-prototype/services/internal/redisx"
	"github.com/buzandanalexandru/eps-prototype/services/internal/sales"
)

type client struct {
	Token   string `json:"token"`
	Session string `json:"session"`
	Pow     string `json:"pow,omitempty"`
	IP      string `json:"ip"`
}

type kit struct {
	Event     string   `json:"eventId"`
	Version   int      `json:"version"`
	Product   string   `json:"productId"`
	Pool      string   `json:"poolId"`
	Capacity  int64    `json:"capacity"`
	Sale      string   `json:"saleId,omitempty"`
	Clients   []client `json:"clients"`
	ExpiresAt int64    `json:"expiresAt"`
}

func main() {
	n := flag.Int("n", 1000, "tokens to mint (one buyer each)")
	capacity := flag.Int64("capacity", 20000, "the pool's capacity")
	withQueue := flag.Bool("queue", false, "add an open sale moment with its waiting room")
	rate := flag.Int("rate", 60000, "the sale's admission rate per minute")
	out := flag.String("out", "/tmp/loadkit.json", "where to write the kit")
	life := flag.Duration("life", 5*time.Minute, "token life (at most 5 minutes)")
	pow := flag.Bool("pow", true, "with -queue: solve each client's proof of work (about 10 ms each)")
	flag.Parse()
	if env := os.Getenv("EPS_ENV"); env != "" && env != "dev" {
		log.Fatal("loadkit runs only with EPS_ENV=dev (it writes into the database directly)")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	org, brand, event, product, poolID := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	slug := "load" + strings.ReplaceAll(org[:8], "-", "")
	must := func(err error) {
		if err != nil {
			log.Fatal(err)
		}
	}
	_, err = pool.Exec(ctx, "insert into control.organisers (id, slug, name) values ($1, $2, 'Load test')", org, slug)
	must(err)
	_, err = pool.Exec(ctx, "insert into control.brands (id, organiser_id, slug, name) values ($1, $2, $3, 'Load test')", brand, org, slug+"-b")
	must(err)
	_, err = pool.Exec(ctx, "insert into control.events (id, organiser_id, brand_id, slug, starts_at, ends_at) values ($1, $2, $3, $4, '2027-07-15T14:00Z', '2027-07-19T00:00Z')", event, org, brand, slug)
	must(err)
	step := map[string]any{"index": 0, "mode": "by_date", "fromAt": nil, "afterSold": nil, "faceMinor": 10000, "allInMinor": 10250, "fanFeeMinor": 250, "organiserNetMinor": 10000}
	doc := map[string]any{
		"schemaVersion": 1, "organiserId": org, "brandId": brand, "eventId": event, "version": 1, "snapshotId": uuid.NewString(),
		"publishedAt": time.Now().UTC().Format(time.RFC3339), "currency": "RON", "timezone": "Europe/Bucharest", "isSample": false,
		"fee":   map[string]any{"policy": "pass_on", "bps": 250, "fixedMinor": 0},
		"event": map[string]any{"slug": slug, "name": map[string]string{"ro": "Test de încărcare", "en": "Load test"}, "description": map[string]string{"ro": "", "en": ""}, "startsAt": "2027-07-15T14:00:00.000Z", "endsAt": "2027-07-19T00:00:00.000Z", "timezone": "Europe/Bucharest", "venue": nil, "days": []any{}, "lineup": []any{}},
		"rules": map[string]any{"minAge": nil, "consentUnderAge": nil, "maxPerAccount": nil, "holdMinutes": 8, "checkoutQuestions": map[string]any{"afterContact": []any{}, "consents": []any{}}},
		"pools": []any{map[string]any{"poolId": poolID, "name": "Arena", "scope": "event", "instances": []any{map[string]any{"instanceId": poolID, "date": nil, "slotStart": nil, "capacity": *capacity}}}},
		"products": []any{map[string]any{"productId": product, "type": "pass", "name": map[string]string{"ro": "Acces general", "en": "General Access"}, "perks": []any{}, "zone": nil,
			"config": map[string]any{}, "named": false, "salesFrom": nil, "salesUntil": nil, "visible": true, "maxPerOrder": 6, "sortOrder": 1,
			"poolIds": []string{poolID}, "requires": []any{}, "priceSteps": []any{step}, "instalments": nil}},
	}
	b, _ := json.Marshal(doc)
	sum := sha256.Sum256(b)
	_, err = pool.Exec(ctx, `insert into control.sale_snapshots (id, organiser_id, event_id, version, schema_version, document_text, sha256, terms_sha256)
		values ($1, $2, $3, 1, 1, $4, $5, $5)`, doc["snapshotId"], org, event, string(b), hex.EncodeToString(sum[:]))
	must(err)
	_, err = pool.Exec(ctx, "update control.events set live_snapshot_version = 1, status = 'published' where id = $1", event)
	must(err)
	k := kit{Event: event, Version: 1, Product: product, Pool: poolID, Capacity: *capacity}
	if *withQueue {
		rdb, err := redisx.Open(os.Getenv("REDIS_URL"))
		must(err)
		st := sales.Defaults(time.Now().Add(-time.Minute).Truncate(time.Second))
		st.RatePerMinute, st.ActiveLimit, st.CapPerOrder, st.CapPerPerson = *rate, 100000, 6, 6
		sv := &sales.Service{Pool: pool, Redis: rdb, MACKey: []byte(os.Getenv("SNAPSHOT_CACHE_KEY"))}
		x, err := sv.Create(ctx, org, event, "Load test sale", "loadkit", st)
		must(err)
		k.Sale = x.ID
	}
	// A signing key the core trusts (as if the web app published it), then the tokens.
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	kid := "loadkit-" + uuid.NewString()[:8]
	jwk, _ := json.Marshal(map[string]string{"kty": "OKP", "crv": "Ed25519", "x": base64.RawURLEncoding.EncodeToString(pub)})
	_, err = pool.Exec(ctx, `insert into control."jwks" ("id", "publicKey", "privateKey", "createdAt", "expiresAt", "alg", "crv") values ($1, $2, 'not-stored', now(), now() + interval '1 day', 'EdDSA', 'Ed25519')`, kid, string(jwk))
	must(err)
	if k.Sale != "" {
		// The gateway reads the token keys from Redis (the core republishes them every minute).
		rdb, err := redisx.Open(os.Getenv("REDIS_URL"))
		must(err)
		must(auth.PublishKeys(ctx, auth.DBKeySource{Pool: pool}, rdb, []byte(os.Getenv("SNAPSHOT_CACHE_KEY"))))
	}
	iss := auth.IssuersFromEnv(os.Getenv)[0]
	exp := time.Now().Add(min(*life, 5*time.Minute))
	k.ExpiresAt = exp.Unix()
	for i := 0; i < *n; i++ {
		sid := uuid.NewString()
		tok := jwt.NewWithClaims(jwt.SigningMethodEdDSA, jwt.MapClaims{"iss": iss, "aud": auth.Audience, "sub": "guest:" + uuid.NewString(), "kind": "guest", "sid": sid,
			"jti": uuid.NewString(), "cip": fmt.Sprintf("10.%d.%d.%d", 64+i/65536%64, i/256%256, i%256), "iat": time.Now().Unix(), "exp": exp.Unix()})
		tok.Header["kid"] = kid
		s, err := tok.SignedString(priv)
		must(err)
		c := client{Token: s, Session: hex.EncodeToString([]byte(sid))[:48], IP: fmt.Sprintf("10.%d.%d.%d", 64+i/65536%64, i/256%256, i%256)}
		if k.Sale != "" && *pow {
			c.Pow = queue.SolvePow(k.Sale, queue.SessionHash(c.Session))
		}
		k.Clients = append(k.Clients, c)
	}
	f, err := os.Create(*out)
	must(err)
	must(json.NewEncoder(f).Encode(k))
	must(f.Close())
	fmt.Printf("loadkit: event %s, sale %q, %d tokens until %s -> %s\n", event, k.Sale, *n, exp.Format(time.TimeOnly), *out)
}
