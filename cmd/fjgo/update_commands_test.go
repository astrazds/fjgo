package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestUpdateYesRejectsTamperedArchive(t *testing.T) {
	bin := buildUpdateBinary(t)
	original, err := os.ReadFile(bin)
	if err != nil {
		t.Fatal(err)
	}
	archiveName := fmt.Sprintf("fjgo_v9.9.9_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	genuine := gzipTar(t, "fjgo", []byte("genuine-fjgo"))
	tampered := gzipTar(t, "fjgo", []byte("tampered-fjgo"))
	checksums := sha256Line(genuine, archiveName)

	server := updateReleaseServer(t, archiveName, tampered, checksums)
	out, err := runUpdateBinary(bin, server.URL)
	if err == nil {
		t.Fatalf("update exit = 0, want checksum failure\n%s", out)
	}
	want := "checksum mismatch for " + archiveName
	if !bytes.Contains(out, []byte(want)) {
		t.Fatalf("output missing %q:\n%s", want, out)
	}
	got, err := os.ReadFile(bin)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, original) {
		t.Fatal("tampered archive replaced the binary")
	}
}

func TestUpdateYesRejectsMissingChecksum(t *testing.T) {
	bin := buildUpdateBinary(t)
	original, err := os.ReadFile(bin)
	if err != nil {
		t.Fatal(err)
	}
	archiveName := fmt.Sprintf("fjgo_v9.9.9_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	archive := gzipTar(t, "fjgo", []byte("unverified-fjgo"))

	server := updateReleaseServer(t, archiveName, archive, "")
	out, err := runUpdateBinary(bin, server.URL)
	if err == nil {
		t.Fatalf("update exit = 0, want missing checksum failure\n%s", out)
	}
	want := "release does not include checksums.txt"
	if !bytes.Contains(out, []byte(want)) {
		t.Fatalf("output missing %q:\n%s", want, out)
	}
	got, err := os.ReadFile(bin)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, original) {
		t.Fatal("archive replaced the binary without checksums.txt")
	}
}

func TestUpdateYesRejectsChecksumLineMissing(t *testing.T) {
	bin := buildUpdateBinary(t)
	original, err := os.ReadFile(bin)
	if err != nil {
		t.Fatal(err)
	}
	archiveName := fmt.Sprintf("fjgo_v9.9.9_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	archive := gzipTar(t, "fjgo", []byte("unlisted-fjgo"))
	checksums := sha256Line(archive, "other.tar.gz")

	server := updateReleaseServer(t, archiveName, archive, checksums)
	out, err := runUpdateBinary(bin, server.URL)
	if err == nil {
		t.Fatalf("update exit = 0, want missing checksum line\n%s", out)
	}
	want := "release checksums do not contain " + archiveName
	if !bytes.Contains(out, []byte(want)) {
		t.Fatalf("output missing %q:\n%s", want, out)
	}
	got, err := os.ReadFile(bin)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, original) {
		t.Fatal("unlisted archive replaced the binary")
	}
}

func TestUpdateYesInstallsMatchingChecksum(t *testing.T) {
	bin := buildUpdateBinary(t)
	archiveName := fmt.Sprintf("fjgo_v9.9.9_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	payload := []byte("genuine-fjgo")
	archive := gzipTar(t, "fjgo", payload)
	checksums := sha256Line(archive, archiveName)

	server := updateReleaseServer(t, archiveName, archive, checksums)
	out, err := runUpdateBinary(bin, server.URL)
	if err != nil {
		t.Fatalf("update error = %v\n%s", err, out)
	}
	got, err := os.ReadFile(bin)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("installed bytes = %q, want %q", got, payload)
	}
}

func buildUpdateBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "fjgo")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

func runUpdateBinary(bin, base string) ([]byte, error) {
	cmd := exec.Command(bin, "update", "--yes", "--base-url", base+"/api/v1", "--repo", "astra/fjgo")
	return cmd.CombinedOutput()
}

func updateReleaseServer(t *testing.T, archiveName string, archive []byte, checksums string) *httptest.Server {
	t.Helper()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/repos/astra/fjgo/releases/latest":
			assets := []map[string]any{{
				"name":                 archiveName,
				"browser_download_url": server.URL + "/downloads/" + archiveName,
				"size":                 len(archive),
			}}
			if checksums != "" {
				assets = append(assets, map[string]any{
					"name":                 "checksums.txt",
					"browser_download_url": server.URL + "/downloads/checksums.txt",
					"size":                 len(checksums),
				})
			}
			if err := json.NewEncoder(w).Encode(map[string]any{
				"tag_name": "v9.9.9",
				"assets":   assets,
			}); err != nil {
				t.Errorf("encode release: %v", err)
			}
		case "/downloads/" + archiveName:
			if _, err := w.Write(archive); err != nil {
				t.Errorf("write archive: %v", err)
			}
		case "/downloads/checksums.txt":
			if _, err := w.Write([]byte(checksums)); err != nil {
				t.Errorf("write checksums: %v", err)
			}
		default:
			http.NotFound(w, r)
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func gzipTar(t *testing.T, name string, contents []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(contents))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(contents); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func sha256Line(archive []byte, archiveName string) string {
	sum := sha256.Sum256(archive)
	return hex.EncodeToString(sum[:]) + "  ./" + archiveName + "\n"
}
