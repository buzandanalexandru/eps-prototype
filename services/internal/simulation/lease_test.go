package simulation_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/buzandanalexandru/eps-prototype/services/internal/simulation"
)

// fakeLeases is a minimal coordination.k8s.io API with optimistic concurrency.
type fakeLeases struct {
	mu  sync.Mutex
	obj map[string]any
	rv  int
}

func (f *fakeLeases) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("Authorization") != "Bearer tok" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	switch r.Method {
	case http.MethodGet:
		if f.obj == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(f.obj)
	case http.MethodPost, http.MethodPut:
		b, _ := io.ReadAll(r.Body)
		var o map[string]any
		_ = json.Unmarshal(b, &o)
		md := o["metadata"].(map[string]any)
		if r.Method == http.MethodPost && f.obj != nil {
			w.WriteHeader(http.StatusConflict)
			return
		}
		if r.Method == http.MethodPut && md["resourceVersion"] != strconv.Itoa(f.rv) {
			w.WriteHeader(http.StatusConflict)
			return
		}
		f.rv++
		md["resourceVersion"] = strconv.Itoa(f.rv)
		f.obj = o
		_ = json.NewEncoder(w).Encode(o)
	}
}

// Review D-b: one rehearsal at a time on the whole cluster.
func TestClusterLease(t *testing.T) {
	srv := httptest.NewServer(&fakeLeases{})
	defer srv.Close()
	now := time.Now()
	mk := func(holder string) *simulation.K8sLease {
		return &simulation.K8sLease{BaseURL: srv.URL, Namespace: "eps-sim", Name: "eps-simulator", Holder: holder, Duration: 30 * time.Second,
			Token: func() (string, error) { return "tok", nil }, HTTP: srv.Client(), Now: func() time.Time { return now }}
	}
	ctx := context.Background()
	a, b := mk("eps-pr-1"), mk("eps-prod")
	if ok, err := a.Acquire(ctx); err != nil || !ok {
		t.Fatalf("A acquires: %v %v", ok, err)
	}
	if ok, err := b.Acquire(ctx); err != nil || ok {
		t.Fatalf("B must wait while A holds it: %v %v", ok, err)
	}
	if ok, _ := a.Acquire(ctx); !ok {
		t.Fatal("A renews")
	}
	now = now.Add(31 * time.Second) // A's simulator died: the lease expires
	if ok, _ := b.Acquire(ctx); !ok {
		t.Fatal("B takes an expired lease")
	}
	b.Release(ctx)
	if ok, _ := a.Acquire(ctx); !ok {
		t.Fatal("a released lease is free")
	}
}
