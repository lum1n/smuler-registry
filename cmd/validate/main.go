package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	semverPattern = regexp.MustCompile(`^\d+\.\d+\.\d+(-[a-zA-Z0-9.]+)?$`)
	idPattern     = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	sha256Pattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

var allowedHosts = map[string]bool{
	"github.com":                 true,
	"objects.githubusercontent.com": true,
}

type registryIndex struct {
	Version string          `json:"version"`
	Plugins []registryEntry `json:"plugins"`
	Themes  []registryEntry `json:"themes"`
}

type registryEntry struct {
	ID                 string   `json:"id"`
	Name               string   `json:"name"`
	Version            string   `json:"version"`
	Description        string   `json:"description"`
	Author             string   `json:"author,omitempty"`
	URL                string   `json:"url"`
	PublicKey          string   `json:"publicKey"`
	SHA256             string   `json:"sha256"`
	MinimumHostVersion string   `json:"minimumHostVersion,omitempty"`
	Capabilities       []string `json:"capabilities,omitempty"`
	Category           string   `json:"category,omitempty"`
	Homepage           string   `json:"homepage,omitempty"`
	PreviewURL         string   `json:"previewUrl,omitempty"`
}

type pluginManifest struct {
	ID                 string        `json:"id"`
	Name               string        `json:"name"`
	Version            string        `json:"version"`
	Description        string        `json:"description"`
	Executable         string        `json:"executable"`
	MinimumHostVersion string        `json:"minimumHostVersion"`
	Capabilities       []string      `json:"capabilities,omitempty"`
	Category           string        `json:"category,omitempty"`
	Preview            string        `json:"preview,omitempty"`
	TokensFile         string        `json:"tokensFile,omitempty"`
	Author             string        `json:"author,omitempty"`
	PublicKey          string        `json:"publicKey,omitempty"`
	Signatures         *manifestSigs `json:"signatures,omitempty"`
}

type manifestSigs struct {
	Manifest string `json:"manifest,omitempty"`
	Binary   string `json:"binary,omitempty"`
	Bundle   string `json:"bundle,omitempty"`
}

func main() {
	indexPath := flag.String("index", "", "path to registry.json")
	entryPath := flag.String("entry", "", "path to a single submission entry JSON")
	skipFetch := flag.Bool("skip-fetch", false, "skip archive download (schema-only)")
	flag.Parse()

	if *indexPath != "" {
		if err := validateIndex(*indexPath, *skipFetch); err != nil {
			fmt.Fprintf(os.Stderr, "validation failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("registry index valid")
		return
	}

	if *entryPath != "" {
		data, err := os.ReadFile(*entryPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read entry: %v\n", err)
			os.Exit(1)
		}
		var entry registryEntry
		if err := json.Unmarshal(data, &entry); err != nil {
			fmt.Fprintf(os.Stderr, "parse entry: %v\n", err)
			os.Exit(1)
		}
		if err := validateEntry(entry, *skipFetch); err != nil {
			fmt.Fprintf(os.Stderr, "validation failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("entry %s@%s valid\n", entry.ID, entry.Version)
		return
	}

	fmt.Fprintln(os.Stderr, "usage: validate --index registry.json | --entry submission.json")
	os.Exit(1)
}

func validateIndex(path string, skipFetch bool) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var idx registryIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	if idx.Version == "" {
		return fmt.Errorf("missing version")
	}

	seenPlugins := map[string]string{}
	for _, e := range idx.Plugins {
		if prev, ok := seenPlugins[e.ID]; ok {
			return fmt.Errorf("duplicate plugin id %q (%s and %s); index keeps one version per id", e.ID, prev, e.Version)
		}
		seenPlugins[e.ID] = e.Version
		if err := validateEntry(e, skipFetch); err != nil {
			return fmt.Errorf("plugin %s: %w", e.ID, err)
		}
	}
	seenThemes := map[string]string{}
	for _, e := range idx.Themes {
		if prev, ok := seenThemes[e.ID]; ok {
			return fmt.Errorf("duplicate theme id %q (%s and %s); index keeps one version per id", e.ID, prev, e.Version)
		}
		seenThemes[e.ID] = e.Version
		if err := validateEntry(e, skipFetch); err != nil {
			return fmt.Errorf("theme %s: %w", e.ID, err)
		}
	}
	return nil
}

func validateEntry(e registryEntry, skipFetch bool) error {
	if !idPattern.MatchString(e.ID) {
		return fmt.Errorf("invalid id %q", e.ID)
	}
	if e.Name == "" {
		return fmt.Errorf("missing name")
	}
	if !semverPattern.MatchString(e.Version) {
		return fmt.Errorf("invalid semver version %q", e.Version)
	}
	if e.Description == "" {
		return fmt.Errorf("missing description")
	}
	if e.PublicKey == "" {
		return fmt.Errorf("missing publicKey")
	}
	if e.SHA256 == "" || !sha256Pattern.MatchString(strings.ToLower(e.SHA256)) {
		return fmt.Errorf("missing or invalid sha256")
	}
	if e.URL == "" {
		return fmt.Errorf("missing url")
	}
	if err := validateURL(e.URL); err != nil {
		return err
	}
	if skipFetch {
		return nil
	}
	return validateArchive(e)
}

func validateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("url must use https")
	}
	host := strings.ToLower(u.Hostname())
	if !allowedHosts[host] {
		return fmt.Errorf("url host %q not in allowlist", host)
	}
	return nil
}

func validateArchive(e registryEntry) error {
	resp, err := http.Get(e.URL)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download HTTP %d", resp.StatusCode)
	}

	archiveData, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read archive: %w", err)
	}

	sum := sha256.Sum256(archiveData)
	got := hex.EncodeToString(sum[:])
	want := strings.ToLower(e.SHA256)
	if got != want {
		return fmt.Errorf("sha256 mismatch: got %s want %s", got, want)
	}

	tmpDir, err := os.MkdirTemp("", "smuler-validate-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	if err := extractArchive(e.URL, archiveData, tmpDir); err != nil {
		return fmt.Errorf("extract: %w", err)
	}

	manifestPath := findManifest(tmpDir)
	if manifestPath == "" {
		return fmt.Errorf("no manifest.json in archive")
	}
	manifestDir := filepath.Dir(manifestPath)
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return err
	}

	var m pluginManifest
	if err := json.Unmarshal(manifestData, &m); err != nil {
		return fmt.Errorf("invalid manifest: %w", err)
	}

	if m.ID != e.ID {
		return fmt.Errorf("manifest id %q != entry id %q", m.ID, e.ID)
	}
	if m.Version != e.Version {
		return fmt.Errorf("manifest version %q != entry version %q", m.Version, e.Version)
	}
	if m.PublicKey != e.PublicKey {
		return fmt.Errorf("manifest publicKey != entry publicKey")
	}

	if m.Executable != "" {
		return verifyPluginSignature(manifestData, &m, manifestDir)
	}
	return verifyThemeSignature(manifestData, &m, manifestDir)
}

func verifyPluginSignature(manifestData []byte, m *pluginManifest, dir string) error {
	if m.PublicKey == "" || m.Signatures == nil || m.Signatures.Manifest == "" {
		return fmt.Errorf("unsigned plugin")
	}
	pubKey, err := decodePublicKey(m.PublicKey)
	if err != nil {
		return err
	}
	manifestBytes, err := manifestSigningPayload(manifestData)
	if err != nil {
		return fmt.Errorf("manifest signing payload: %w", err)
	}
	if !verifySig(pubKey, manifestBytes, m.Signatures.Manifest) {
		return fmt.Errorf("manifest signature mismatch")
	}
	if m.Signatures.Binary != "" && m.Executable != "" {
		binaryData, err := os.ReadFile(filepath.Join(dir, m.Executable))
		if err != nil {
			return fmt.Errorf("binary not found")
		}
		if !verifySig(pubKey, binaryData, m.Signatures.Binary) {
			return fmt.Errorf("binary signature mismatch")
		}
	}
	return nil
}

func verifyThemeSignature(manifestData []byte, m *pluginManifest, dir string) error {
	if m.PublicKey == "" || m.Signatures == nil || m.Signatures.Manifest == "" {
		return fmt.Errorf("unsigned theme")
	}
	pubKey, err := decodePublicKey(m.PublicKey)
	if err != nil {
		return err
	}
	manifestBytes, err := manifestSigningPayload(manifestData)
	if err != nil {
		return fmt.Errorf("manifest signing payload: %w", err)
	}
	if !verifySig(pubKey, manifestBytes, m.Signatures.Manifest) {
		return fmt.Errorf("manifest signature mismatch")
	}
	tokensFile := m.TokensFile
	if tokensFile == "" {
		tokensFile = "tokens.json"
	}
	if m.Signatures.Bundle != "" {
		bundleData, err := os.ReadFile(filepath.Join(dir, tokensFile))
		if err != nil {
			return fmt.Errorf("tokens file not found")
		}
		if !verifySig(pubKey, bundleData, m.Signatures.Bundle) {
			return fmt.Errorf("bundle signature mismatch")
		}
	}
	return nil
}

// manifestSigningPayload reconstructs the bytes smuler signs: compact JSON of the
// manifest object with the signatures field removed, preserving field order at
// every nesting level to match smuler plugin sign/publish.
func manifestSigningPayload(manifestData []byte) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(manifestData))
	token, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if token != json.Delim('{') {
		return nil, fmt.Errorf("manifest root must be an object")
	}

	var buf bytes.Buffer
	buf.WriteByte('{')
	first := true
	for dec.More() {
		keyToken, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := keyToken.(string)
		if !ok {
			return nil, fmt.Errorf("manifest object keys must be strings")
		}
		value, err := compactPreserveOrder(dec)
		if err != nil {
			return nil, err
		}
		if key == "signatures" {
			continue
		}
		if !first {
			buf.WriteByte(',')
		}
		first = false
		keyJSON, err := json.Marshal(key)
		if err != nil {
			return nil, err
		}
		buf.Write(keyJSON)
		buf.WriteByte(':')
		buf.Write(value)
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

func compactPreserveOrder(dec *json.Decoder) ([]byte, error) {
	token, err := dec.Token()
	if err != nil {
		return nil, err
	}

	switch v := token.(type) {
	case json.Delim:
		switch v {
		case '{':
			var buf bytes.Buffer
			buf.WriteByte('{')
			first := true
			for dec.More() {
				keyToken, err := dec.Token()
				if err != nil {
					return nil, err
				}
				key, ok := keyToken.(string)
				if !ok {
					return nil, fmt.Errorf("object keys must be strings")
				}
				value, err := compactPreserveOrder(dec)
				if err != nil {
					return nil, err
				}
				if !first {
					buf.WriteByte(',')
				}
				first = false
				keyJSON, err := json.Marshal(key)
				if err != nil {
					return nil, err
				}
				buf.Write(keyJSON)
				buf.WriteByte(':')
				buf.Write(value)
			}
			if _, err := dec.Token(); err != nil {
				return nil, err
			}
			buf.WriteByte('}')
			return buf.Bytes(), nil
		case '[':
			var buf bytes.Buffer
			buf.WriteByte('[')
			first := true
			for dec.More() {
				value, err := compactPreserveOrder(dec)
				if err != nil {
					return nil, err
				}
				if !first {
					buf.WriteByte(',')
				}
				first = false
				buf.Write(value)
			}
			if _, err := dec.Token(); err != nil {
				return nil, err
			}
			buf.WriteByte(']')
			return buf.Bytes(), nil
		default:
			return nil, fmt.Errorf("unexpected delimiter %q", v)
		}
	default:
		return json.Marshal(v)
	}
}

func decodePublicKey(b64 string) (ed25519.PublicKey, error) {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil || len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("invalid public key")
	}
	return ed25519.PublicKey(raw), nil
}

func verifySig(pub ed25519.PublicKey, data []byte, sigB64 string) bool {
	sig, err := base64.StdEncoding.DecodeString(sigB64)
	if err != nil {
		return false
	}
	hash := sha256.Sum256(data)
	return ed25519.Verify(pub, hash[:], sig)
}

func extractArchive(url string, data []byte, dest string) error {
	if strings.HasSuffix(strings.ToLower(url), ".zip") {
		return extractZip(data, dest)
	}
	return extractTarGz(data, dest)
}

func extractZip(data []byte, dest string) error {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	for _, f := range r.File {
		path := filepath.Join(dest, f.Name)
		if !strings.HasPrefix(filepath.Clean(path), filepath.Clean(dest)+string(os.PathSeparator)) {
			continue
		}
		if f.FileInfo().IsDir() {
			os.MkdirAll(path, 0755)
			continue
		}
		os.MkdirAll(filepath.Dir(path), 0755)
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			rc.Close()
			return err
		}
		io.Copy(out, rc)
		rc.Close()
		out.Close()
	}
	return nil
}

func extractTarGz(data []byte, dest string) error {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		path := filepath.Join(dest, header.Name)
		if !strings.HasPrefix(filepath.Clean(path), filepath.Clean(dest)+string(os.PathSeparator)) {
			continue
		}
		switch header.Typeflag {
		case tar.TypeDir:
			os.MkdirAll(path, 0755)
		case tar.TypeReg:
			os.MkdirAll(filepath.Dir(path), 0755)
			out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			io.Copy(out, tr)
			out.Close()
		}
	}
	return nil
}

func findManifest(dir string) string {
	var result string
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || result != "" {
			return nil
		}
		if info.Name() == "manifest.json" {
			result = path
		}
		return nil
	})
	return result
}
