package repo2skill

import "testing"

func TestCollectTypeScriptManifestEntrypoints(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "package.json", `{"name":"ts-lib","version":"1.0.0","main":"dist/index.js","types":"src/public.ts","exports":{".":{"types":"./src/public.ts","import":"./dist/index.js"},"./feature":"./src/feature.ts"}}`)
	writeFile(t, dir, "src/public.ts", "export interface Client {}\nexport const version = '1'\n")
	writeFile(t, dir, "src/feature.ts", "export function makeFeature() { return true }\n")

	p, cleanup, err := Collect(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if !hasEntryPoint(p.EntryPoints, "src/public.ts") || !hasEntryPoint(p.EntryPoints, "src/feature.ts") {
		t.Fatalf("entrypoints: %#v", p.EntryPoints)
	}
	for _, name := range []string{"Client", "version", "makeFeature"} {
		if !hasSymbol(p.PublicAPIs, name) {
			t.Fatalf("missing %s in symbols: %#v", name, p.PublicAPIs)
		}
	}
}

func TestCollectPythonPackageEntrypoint(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "pyproject.toml", "[project]\nname = \"cool-py\"\nversion = \"0.1.0\"\ndescription = \"Cool Python helpers\"\n")
	writeFile(t, dir, "src/cool_py/__init__.py", "__all__ = ['Client', 'parse']\nclass Client: pass\ndef parse(): pass\ndef _private(): pass\n")

	p, cleanup, err := Collect(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if !hasEntryPoint(p.EntryPoints, "src/cool_py/__init__.py") {
		t.Fatalf("entrypoints: %#v", p.EntryPoints)
	}
	if !hasSymbol(p.PublicAPIs, "Client") || !hasSymbol(p.PublicAPIs, "parse") || hasSymbol(p.PublicAPIs, "_private") {
		t.Fatalf("unexpected symbols: %#v", p.PublicAPIs)
	}
	if p.InstallHints[0] != "pip install cool-py" {
		t.Fatalf("install hints: %#v", p.InstallHints)
	}
}

func TestCollectGoExportedSymbols(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module example.com/coolgo\n\ngo 1.22\n")
	writeFile(t, dir, "cool.go", "package coolgo\n\nconst Version = \"1\"\ntype Client struct{}\nfunc NewClient() *Client { return &Client{} }\nfunc hidden() {}\n")

	p, cleanup, err := Collect(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	for _, name := range []string{"Version", "Client", "NewClient"} {
		if !hasSymbol(p.PublicAPIs, name) {
			t.Fatalf("missing %s in symbols: %#v", name, p.PublicAPIs)
		}
	}
	if hasSymbol(p.PublicAPIs, "hidden") {
		t.Fatalf("unexpected private symbol: %#v", p.PublicAPIs)
	}
}

func TestCollectRustCrateEntrypoint(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "Cargo.toml", "[package]\nname = \"cool-rs\"\nversion = \"0.1.0\"\ndescription = \"Cool Rust crate\"\n")
	writeFile(t, dir, "src/lib.rs", "pub struct Client;\npub fn connect() {}\nfn internal() {}\n")

	p, cleanup, err := Collect(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if !hasEntryPoint(p.EntryPoints, "src/lib.rs") {
		t.Fatalf("entrypoints: %#v", p.EntryPoints)
	}
	if !hasSymbol(p.PublicAPIs, "Client") || !hasSymbol(p.PublicAPIs, "connect") || hasSymbol(p.PublicAPIs, "internal") {
		t.Fatalf("unexpected symbols: %#v", p.PublicAPIs)
	}
}

func hasEntryPoint(items []EntryPoint, path string) bool {
	for _, item := range items {
		if item.Path == path {
			return true
		}
	}
	return false
}
