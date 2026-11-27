package db

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestRedactURLAndKeyValue(t *testing.T) {
	cases := map[string]string{
		"postgres://app:s3cr3t-pa55@ep-x.eu-central-1.aws.neon.tech/neondb?sslmode=require": "postgres://app:xxxxx@ep-x.eu-central-1.aws.neon.tech/neondb?sslmode=require",
		"host=db.example user=app password=s3cr3t-pa55 dbname=x":                           "host=db.example user=app password=xxxxx dbname=x",
		"password='s3cr3t pa55' host=h":                                                    "password=xxxxx host=h",
		"failed: postgresql://u:s3cr3t-pa55@h:5432/d: dial error":                          "failed: postgresql://u:xxxxx@h:5432/d: dial error",
	}
	for in, want := range cases {
		if got := Redact(in); got != want {
			t.Errorf("Redact(%q) = %q, want %q", in, got, want)
		}
	}
}

// Scenario "Logging a connection error": the log line names the host and the error but not the
// password or the full connection string.
func TestConnectionErrorLogHasHostButNoSecret(t *testing.T) {
	const password = "s3cr3t-pa55-word"
	dsn := "postgres://app:" + password + "@127.0.0.1:1/neondb?sslmode=disable&connect_timeout=2"
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := Open(ctx, dsn)
	if err == nil {
		t.Fatal("expected a connection error")
	}
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))
	log.Error("postgres connect failed", "host", HostOf(dsn), "error", err.Error())
	line := buf.String()
	if strings.Contains(line, password) {
		t.Fatalf("log line leaks the password: %s", line)
	}
	if strings.Contains(line, dsn) || strings.Contains(line, "app:"+password) {
		t.Fatalf("log line holds the connection string: %s", line)
	}
	if !strings.Contains(line, "127.0.0.1:1") {
		t.Fatalf("log line should name the host: %s", line)
	}
	if !strings.Contains(err.Error(), "127.0.0.1") {
		t.Fatalf("error should name the host: %v", err)
	}

	// An unparsable string never echoes its contents.
	_, err = Open(ctx, "postgres://app:"+password+"@[bad")
	if err == nil || strings.Contains(err.Error(), password) {
		t.Fatalf("parse error leaks or is missing: %v", err)
	}
}
