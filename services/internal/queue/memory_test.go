package queue_test

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/buzandanalexandru/eps-prototype/services/internal/onsaletest"
	"github.com/buzandanalexandru/eps-prototype/services/internal/queue"
)

func usedMemory(t *testing.T, env *onsaletest.Env) int64 {
	t.Helper()
	info, err := env.Redis.Info(context.Background(), "memory").Result()
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range strings.Split(info, "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(l), "used_memory:"); ok {
			n, _ := strconv.ParseInt(v, 10, 64)
			return n
		}
	}
	t.Fatal("no used_memory")
	return 0
}

// Review D-h: what one fan in a waiting room costs Redis (place, its hash with signals, its spot
// in line, the address sets, the rate-limit counters and the audit entry until the worker copies
// it), measured over 10,000 fans on distinct addresses with bot defence on.
func TestMemoryPerFan(t *testing.T) {
	if testing.Short() {
		t.Skip("10,000 joins")
	}
	env := onsaletest.Setup(t)
	ctx := context.Background()
	st := env.Open()
	st.BotDefence = true
	st.OpensAt = env.Clock.Now().Add(20 * time.Minute).Truncate(time.Second)
	s := env.NewSale(t, env.Env.NewSale(t, 100, 100), st)
	_, _ = env.Sales.Lookup(ctx, s.ID)
	before := usedMemory(t, env)
	var wg sync.WaitGroup
	sem := make(chan struct{}, 16)
	for i := 0; i < 10000; i++ {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			_, err := env.Engine.Join(ctx, s.ID, queue.JoinInput{Sub: uuid.NewString(), Kind: "account", Session: uuid.NewString() + uuid.NewString(),
				UA: "Mozilla/5.0 Safari", IP: fmt.Sprintf("10.%d.%d.%d", i/65536%256, i/256%256, i%256), Print: "fp-" + strconv.Itoa(i%50)})
			if err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	per := (usedMemory(t, env) - before) / 10000
	t.Logf("%d bytes per fan in the pre-queue", per)
	if per > 1600 {
		t.Fatalf("%d bytes per fan (budget 1,600: 100,000 fans in about 160 MB of the 1 GiB limit)", per)
	}
}
