package tenancy_test

import (
	"testing"

	"github.com/buzandanalexandru/eps-prototype/services/internal/testdb"
)

func TestMain(m *testing.M) { testdb.Main(m) }

const (
	orgA = "00000000-0000-4000-8000-00000000000a"
	orgB = "00000000-0000-4000-8000-00000000000b"
)
