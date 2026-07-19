package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

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
}

type pluginManifest struct {
	ID                 string        `json:"id"`
	Name               string        `json:"name"`
	Version            string        `json:"version"`
	Description        string        `json:"description"`
	Executable         string        `json:"executable"`
	MinimumHostVersion string        `json:"minimumHostVersion"`
	Capabilities       []string      `json:"capabilities,omitempty"`
	Author             string        `json:"author,omitempty"`
	PublicKey          string        `json:"publicKey,omitempty"`
	Signatures         *manifestSigs `json:"signatures,omitempty"`
}

type manifestSigs struct {
	Manifest string `json:"manifest,omitempty"`
	Binary   string `json:"binary,omitempty"`
}

func main() {
	outDir := flag.String("out", "releases/plugins-v0.1.0", "output directory for archives")
	submissionsDir := flag.String("submissions", "submissions/plugins", "submission JSON directory")
	releaseRepo := flag.String("repo", "lum1n/smuler-registry", "GitHub repo hosting release assets")
	releaseTag := flag.String("tag", "plugins-v0.1.0", "GitHub release tag")
	flag.Parse()

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		fatal(err)
	}
	publicKeyB64 := base64.StdEncoding.EncodeToString(pub)

	entries, err := loadSubmissions(*submissionsDir)
	if err != nil {
		fatal(err)
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fatal(err)
	}

	for _, entry := range entries {
		entry.PublicKey = publicKeyB64
		entry.URL = fmt.Sprintf("https://github.com/%s/releases/download/%s/%s-%s.tar.gz",
			*releaseRepo, *releaseTag, entry.ID, entry.Version)
		if entry.MinimumHostVersion == "" {
			entry.MinimumHostVersion = "0.1.0"
		}

		archiveName := fmt.Sprintf("%s-%s.tar.gz", entry.ID, entry.Version)
		archivePath := filepath.Join(*outDir, archiveName)
		archiveData, err := buildArchive(&entry, priv, publicKeyB64)
		if err != nil {
			fatal(fmt.Errorf("%s: %w", entry.ID, err))
		}
		if err := os.WriteFile(archivePath, archiveData, 0o644); err != nil {
			fatal(err)
		}

		sum := sha256.Sum256(archiveData)
		entry.SHA256 = hex.EncodeToString(sum[:])

		submissionPath := filepath.Join(*submissionsDir, archiveName[:len(archiveName)-7]+".json")
		data, err := json.MarshalIndent(entry, "", "  ")
		if err != nil {
			fatal(err)
		}
		data = append(data, '\n')
		if err := os.WriteFile(submissionPath, data, 0o644); err != nil {
			fatal(err)
		}
		fmt.Printf("generated %s (%s)\n", archivePath, entry.SHA256)
	}
}

func loadSubmissions(dir string) ([]registryEntry, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	sort.Strings(matches)
	var entries []registryEntry
	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var entry registryEntry
		if err := json.Unmarshal(data, &entry); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func buildArchive(entry *registryEntry, priv ed25519.PrivateKey, publicKeyB64 string) ([]byte, error) {
	manifest := pluginManifest{
		ID:                 entry.ID,
		Name:               entry.Name,
		Version:            entry.Version,
		Description:        entry.Description,
		Executable:         "plugin",
		MinimumHostVersion: entry.MinimumHostVersion,
		Capabilities:       entry.Capabilities,
		Author:             entry.Author,
		PublicKey:          publicKeyB64,
	}

	sanitized := pluginManifest{
		ID: manifest.ID, Name: manifest.Name, Version: manifest.Version,
		Description: manifest.Description, Executable: manifest.Executable,
		MinimumHostVersion: manifest.MinimumHostVersion, Capabilities: manifest.Capabilities,
		Author: manifest.Author, PublicKey: manifest.PublicKey,
	}
	manifestBytes, err := json.Marshal(sanitized)
	if err != nil {
		return nil, err
	}

	binaryData := []byte("#!/bin/sh\n# Smuler plugin stub\nexit 0\n")
	manifestSig, err := sign(priv, manifestBytes)
	if err != nil {
		return nil, err
	}
	binarySig, err := sign(priv, binaryData)
	if err != nil {
		return nil, err
	}
	manifest.Signatures = &manifestSigs{Manifest: manifestSig, Binary: binarySig}

	finalManifest, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, err
	}
	finalManifest = append(finalManifest, '\n')

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	files := map[string][]byte{
		"manifest.json": finalManifest,
		"plugin":        binaryData,
	}
	for name, content := range files {
		if err := writeTarFile(tw, name, content, 0o755); err != nil {
			return nil, err
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeTarFile(tw *tar.Writer, name string, data []byte, mode int64) error {
	header := &tar.Header{
		Name:    name,
		Mode:    mode,
		Size:    int64(len(data)),
		Format:  tar.FormatGNU,
	}
	if strings.HasPrefix(name, "plugin") {
		header.Mode = 0o755
	}
	if err := tw.WriteHeader(header); err != nil {
		return err
	}
	_, err := tw.Write(data)
	return err
}

func sign(priv ed25519.PrivateKey, data []byte) (string, error) {
	hash := sha256.Sum256(data)
	sig := ed25519.Sign(priv, hash[:])
	return base64.StdEncoding.EncodeToString(sig), nil
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}
