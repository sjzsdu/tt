package cmd

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func TestImageResponseBytesRawBase64(t *testing.T) {
	want := []byte("png-bytes")
	got, err := imageResponseBytes(base64.StdEncoding.EncodeToString(want))
	if err != nil {
		t.Fatalf("imageResponseBytes() error = %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("imageResponseBytes() = %q, want %q", got, want)
	}
}

func TestImageResponseBytesDataURL(t *testing.T) {
	want := []byte("png-bytes")
	got, err := imageResponseBytes("data:image/png;base64," + base64.StdEncoding.EncodeToString(want))
	if err != nil {
		t.Fatalf("imageResponseBytes() error = %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("imageResponseBytes() = %q, want %q", got, want)
	}
}

func TestImageResponseBytesDalleJSON(t *testing.T) {
	want := []byte("png-bytes")
	content := `{"data":[{"b64_json":"` + base64.StdEncoding.EncodeToString(want) + `"}]}`
	got, err := imageResponseBytes(content)
	if err != nil {
		t.Fatalf("imageResponseBytes() error = %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("imageResponseBytes() = %q, want %q", got, want)
	}
}

func TestSaveImageResponseCurrentDir(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(cwd)

	want := []byte("png-bytes")
	if err := saveImageResponse("out.png", base64.StdEncoding.EncodeToString(want)); err != nil {
		t.Fatalf("saveImageResponse() error = %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "out.png"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("saved bytes = %q, want %q", got, want)
	}
}
