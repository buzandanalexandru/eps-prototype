package snapshot_test

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/buzandanalexandru/eps-prototype/services/internal/snapshot"
)

func fixture(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/galaxia.v1.json")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// Scenario "Matches the shared schema": a hand-written snapshot validates against the shared
// JSON Schema and decodes into the Go struct; the web side validates every compiled seed event
// against the same file (apps/web/tests/unit/snapshot-compile.test.ts).
func TestFixtureMatchesTheSharedSchema(t *testing.T) {
	root := loadSchema(t)
	doc := fixture(t)
	if err := validate(root, doc); err != nil {
		t.Fatalf("fixture does not validate: %v", err)
	}
	s, err := snapshot.Parse(doc)
	if err != nil {
		t.Fatal(err)
	}
	if s.Product("0d000000-0000-4000-8000-000000000001").PriceSteps[0].AllInMinor != 55248 {
		t.Fatal("GA all-in not decoded")
	}
	// The Go struct carries every field of the schema: re-encoding validates too.
	again, _ := json.Marshal(s)
	if err := validate(root, again); err != nil {
		t.Fatalf("re-encoded struct does not validate: %v", err)
	}
}

func TestSchemaRejectsMissingField(t *testing.T) {
	root := loadSchema(t)
	var m map[string]any
	_ = json.Unmarshal(fixture(t), &m)
	delete(m["fee"].(map[string]any), "policy")
	b, _ := json.Marshal(m)
	if err := validate(root, b); err == nil || !strings.Contains(err.Error(), "policy") {
		t.Fatalf("missing fee.policy accepted: %v", err)
	}
	_ = json.Unmarshal(fixture(t), &m)
	m["extra"] = true
	b, _ = json.Marshal(m)
	if err := validate(root, b); err == nil {
		t.Fatal("unknown top-level field accepted")
	}
}

// Scenario "Amounts add up": the core refuses a document whose all-in is not face + fan fee.
func TestAllInMustEqualFacePlusFee(t *testing.T) {
	var m map[string]any
	_ = json.Unmarshal(fixture(t), &m)
	step := m["products"].([]any)[0].(map[string]any)["priceSteps"].([]any)[0].(map[string]any)
	step["allInMinor"] = 53900.0
	b, _ := json.Marshal(m)
	if _, err := snapshot.Parse(b); !errors.Is(err, snapshot.ErrInvalid) {
		t.Fatalf("want ErrInvalid, got %v", err)
	}
}
