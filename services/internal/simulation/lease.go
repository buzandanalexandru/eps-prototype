package simulation

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Lease is the cluster-wide right to run a rehearsal (review D-b): every environment shares the
// same nodes, so only one simulator in the cluster drives traffic at a time.
type Lease interface {
	// Acquire takes or renews the lease for this holder; false when another holder has it.
	Acquire(ctx context.Context) (bool, error)
	// Release gives it up.
	Release(ctx context.Context)
}

// K8sLease is a coordination.k8s.io Lease in a namespace every environment's simulator may update
// (the chart's Role and RoleBinding in that namespace), through the API with the pod's own token.
type K8sLease struct {
	BaseURL   string // https://kubernetes.default.svc
	Namespace string
	Name      string
	Holder    string
	Token     func() (string, error)
	HTTP      *http.Client
	Duration  time.Duration
	Now       func() time.Time
}

// InClusterLease builds the lease from the pod's service account (nil outside a cluster).
func InClusterLease(namespace, holder string) (*K8sLease, error) {
	host, port := os.Getenv("KUBERNETES_SERVICE_HOST"), os.Getenv("KUBERNETES_SERVICE_PORT")
	if host == "" || port == "" {
		return nil, errors.New("simulation: not in a cluster")
	}
	const dir = "/var/run/secrets/kubernetes.io/serviceaccount/"
	ca, err := os.ReadFile(dir + "ca.crt")
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(ca)
	return &K8sLease{
		BaseURL: "https://" + host + ":" + port, Namespace: namespace, Name: "eps-simulator", Holder: holder, Duration: 30 * time.Second,
		Token: func() (string, error) {
			b, err := os.ReadFile(dir + "token")
			return strings.TrimSpace(string(b)), err
		},
		HTTP: &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}}},
	}, nil
}

type leaseObj struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Metadata   struct {
		Name            string `json:"name"`
		Namespace       string `json:"namespace,omitempty"`
		ResourceVersion string `json:"resourceVersion,omitempty"`
	} `json:"metadata"`
	Spec struct {
		HolderIdentity       *string `json:"holderIdentity,omitempty"`
		LeaseDurationSeconds *int    `json:"leaseDurationSeconds,omitempty"`
		RenewTime            *string `json:"renewTime,omitempty"`
	} `json:"spec"`
}

const microTime = "2006-01-02T15:04:05.000000Z07:00"

func (l *K8sLease) now() time.Time {
	if l.Now != nil {
		return l.Now()
	}
	return time.Now()
}

func (l *K8sLease) do(ctx context.Context, method, path string, body any) (int, []byte, error) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, l.BaseURL+path, rd)
	if err != nil {
		return 0, nil, err
	}
	tok, err := l.Token()
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	resp, err := l.HTTP.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	return resp.StatusCode, b, nil
}

func (l *K8sLease) path(named bool) string {
	p := "/apis/coordination.k8s.io/v1/namespaces/" + l.Namespace + "/leases"
	if named {
		p += "/" + l.Name
	}
	return p
}

func (l *K8sLease) write(ctx context.Context, cur *leaseObj, holder string, renew time.Time) (bool, error) {
	o := leaseObj{APIVersion: "coordination.k8s.io/v1", Kind: "Lease"}
	o.Metadata.Name, o.Metadata.Namespace = l.Name, l.Namespace
	secs := int(l.Duration.Seconds())
	rt := renew.UTC().Format(microTime)
	o.Spec.HolderIdentity, o.Spec.LeaseDurationSeconds, o.Spec.RenewTime = &holder, &secs, &rt
	method, path := http.MethodPost, l.path(false)
	if cur != nil {
		o.Metadata.ResourceVersion = cur.Metadata.ResourceVersion
		method, path = http.MethodPut, l.path(true)
	}
	code, body, err := l.do(ctx, method, path, o)
	switch {
	case err != nil:
		return false, err
	case code == http.StatusConflict:
		return false, nil // someone else wrote it first
	case code >= 300:
		return false, fmt.Errorf("lease %s: %d %s", method, code, strings.TrimSpace(string(body)))
	}
	return true, nil
}

// Acquire takes the lease when it is free or expired, or renews it for its holder.
func (l *K8sLease) Acquire(ctx context.Context) (bool, error) {
	code, body, err := l.do(ctx, http.MethodGet, l.path(true), nil)
	if err != nil {
		return false, err
	}
	if code == http.StatusNotFound {
		return l.write(ctx, nil, l.Holder, l.now())
	}
	if code != http.StatusOK {
		return false, fmt.Errorf("lease get: %d", code)
	}
	var cur leaseObj
	if err := json.Unmarshal(body, &cur); err != nil {
		return false, err
	}
	holder := ""
	if cur.Spec.HolderIdentity != nil {
		holder = *cur.Spec.HolderIdentity
	}
	if holder != "" && holder != l.Holder && cur.Spec.RenewTime != nil {
		dur := l.Duration
		if cur.Spec.LeaseDurationSeconds != nil {
			dur = time.Duration(*cur.Spec.LeaseDurationSeconds) * time.Second
		}
		if t, err := time.Parse(microTime, *cur.Spec.RenewTime); err == nil && l.now().Before(t.Add(dur)) {
			return false, nil // held by another environment's simulator
		}
	}
	return l.write(ctx, &cur, l.Holder, l.now())
}

// Release frees the lease if this holder has it.
func (l *K8sLease) Release(ctx context.Context) {
	code, body, err := l.do(ctx, http.MethodGet, l.path(true), nil)
	if err != nil || code != http.StatusOK {
		return
	}
	var cur leaseObj
	if json.Unmarshal(body, &cur) != nil || cur.Spec.HolderIdentity == nil || *cur.Spec.HolderIdentity != l.Holder {
		return
	}
	_, _ = l.write(ctx, &cur, "", l.now().Add(-time.Hour))
}
