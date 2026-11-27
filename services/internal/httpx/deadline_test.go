package httpx

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Ordinary requests get a write deadline; server-sent event streams do not, so the server needs no
// global WriteTimeout that would cut a live stream.
func TestWriteDeadlineSparesEventStreams(t *testing.T) {
	slow := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(150 * time.Millisecond)
		_, _ = io.WriteString(w, "late")
	})
	srv := httptest.NewServer(WithWriteDeadline(50*time.Millisecond, IsEventStream)(slow))
	defer srv.Close()

	get := func(accept string) (string, error) {
		req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
		if accept != "" {
			req.Header.Set("Accept", accept)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			return "", err
		}
		defer res.Body.Close()
		b, err := io.ReadAll(res.Body)
		return string(b), err
	}
	if body, err := get(""); err == nil && body == "late" {
		t.Fatal("ordinary request outlived its write deadline")
	}
	if body, err := get("text/event-stream"); err != nil || body != "late" {
		t.Fatalf("event stream cut: %q, %v", body, err)
	}
}
