package main

import (
	"archive/tar"
	"bytes"
	"io"
	"os"
	"os/exec"
	"path"
	"strings"
	"testing"
)

// Scenario "Image contents": a built Go image holds only the binary, CA certificates and the
// distroless base files, and no environment file or key.
//
// Building an image is slow, so the test runs when EPS_IMAGE_TEST=1 (CI's image job sets it), or
// against an already built image named in EPS_IMAGE.
func TestImageHoldsOnlyBinaryAndCACerts(t *testing.T) {
	image := os.Getenv("EPS_IMAGE")
	if image == "" {
		if os.Getenv("EPS_IMAGE_TEST") != "1" {
			t.Skip("set EPS_IMAGE_TEST=1 (builds services/Dockerfile --target core) or EPS_IMAGE=<tag>")
		}
		image = "eps-core:image-test"
		build := exec.Command("docker", "build", "--target", "core", "--build-arg", "COMMIT=image-test", "-t", image, "../..")
		build.Stdout, build.Stderr = os.Stderr, os.Stderr
		if err := build.Run(); err != nil {
			t.Fatalf("docker build: %v", err)
		}
	}
	out, err := exec.Command("docker", "create", image).Output()
	if err != nil {
		t.Fatalf("docker create: %v", err)
	}
	id := strings.TrimSpace(string(out))
	defer exec.Command("docker", "rm", "-f", id).Run()
	tarball, err := exec.Command("docker", "export", id).Output()
	if err != nil {
		t.Fatalf("docker export: %v", err)
	}

	var files []string
	sawBinary, sawCA := false, false
	tr := tar.NewReader(bytes.NewReader(tarball))
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		name := "/" + strings.TrimPrefix(h.Name, "./")
		if h.Typeflag != tar.TypeReg || name == "/.dockerenv" || strings.HasPrefix(name, "/dev/") {
			continue // added by the container runtime at create time, not part of the image
		}
		files = append(files, name)
		base := path.Base(name)
		switch {
		case name == "/core":
			sawBinary = true
		case name == "/etc/ssl/certs/ca-certificates.crt":
			sawCA = true
		case strings.HasPrefix(base, ".env") || strings.HasSuffix(base, ".env"):
			t.Errorf("environment file in image: %s", name)
		case strings.HasSuffix(base, ".key") || strings.HasSuffix(base, ".pem") && !strings.HasPrefix(name, "/etc/ssl/"):
			t.Errorf("key file in image: %s", name)
		case strings.HasPrefix(name, "/src/") || strings.HasPrefix(name, "/go/"):
			t.Errorf("build files in image: %s", name)
		}
		if h.Size < 10<<20 && name != "/core" {
			b, _ := io.ReadAll(tr)
			if bytes.Contains(b, []byte("PRIVATE KEY")) {
				t.Errorf("private key material in %s", name)
			}
		}
		// Only the base's known locations, the binary and the certificates are allowed.
		allowed := name == "/core" || strings.HasPrefix(name, "/etc/") || strings.HasPrefix(name, "/usr/share/") ||
			strings.HasPrefix(name, "/var/lib/dpkg/") || name == "/usr/lib/os-release"
		if !allowed {
			t.Errorf("unexpected file in image: %s", name)
		}
	}
	if !sawBinary || !sawCA {
		t.Fatalf("binary present=%v, CA certificates present=%v (files: %v)", sawBinary, sawCA, files)
	}
}
