package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type registryIndex struct {
	Version     string          `json:"version"`
	Plugins     []registryEntry `json:"plugins"`
	Themes      []registryEntry `json:"themes"`
	GeneratedAt string          `json:"generatedAt,omitempty"`
	Source      string          `json:"source,omitempty"`
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

func main() {
	indexPath := flag.String("index", "registry.json", "path to registry.json")
	submissionsDir := flag.String("submissions", "submissions", "submissions root directory")
	keepSubmissions := flag.Bool("keep-submissions", false, "keep submission files after merge")
	dryRun := flag.Bool("dry-run", false, "print actions without writing")
	flag.Parse()

	merged, err := mergeSubmissions(*indexPath, *submissionsDir, *keepSubmissions, *dryRun)
	if err != nil {
		fmt.Fprintf(os.Stderr, "merge failed: %v\n", err)
		os.Exit(1)
	}
	if merged == 0 {
		fmt.Println("no submission files to merge")
		return
	}
	fmt.Printf("merged %d submission(s) into %s\n", merged, *indexPath)
}

func mergeSubmissions(indexPath, submissionsDir string, keepSubmissions, dryRun bool) (int, error) {
	index, err := loadIndex(indexPath)
	if err != nil {
		return 0, err
	}

	type pending struct {
		kind string
		path string
		entry registryEntry
	}

	var pendingEntries []pending
	for _, kind := range []string{"plugins", "themes"} {
		dir := filepath.Join(submissionsDir, kind)
		matches, err := filepath.Glob(filepath.Join(dir, "*.json"))
		if err != nil {
			return 0, err
		}
		sort.Strings(matches)
		for _, path := range matches {
			entry, err := loadEntry(path)
			if err != nil {
				return 0, fmt.Errorf("%s: %w", path, err)
			}
			pendingEntries = append(pendingEntries, pending{kind: kind, path: path, entry: entry})
		}
	}

	if len(pendingEntries) == 0 {
		return 0, nil
	}

	merged := 0
	for _, item := range pendingEntries {
		switch item.kind {
		case "plugins":
			if err := upsertEntry(&index.Plugins, "plugin", item.entry); err != nil {
				return merged, fmt.Errorf("%s: %w", item.path, err)
			}
		case "themes":
			if err := upsertEntry(&index.Themes, "theme", item.entry); err != nil {
				return merged, fmt.Errorf("%s: %w", item.path, err)
			}
		}

		if dryRun {
			fmt.Printf("would merge %s -> %s (%s@%s)\n", item.path, indexPath, item.entry.ID, item.entry.Version)
			if !keepSubmissions {
				fmt.Printf("would remove %s\n", item.path)
			}
		} else if !keepSubmissions {
			if err := os.Remove(item.path); err != nil {
				return merged, fmt.Errorf("remove %s: %w", item.path, err)
			}
		}
		merged++
	}

	sortEntries(index.Plugins)
	sortEntries(index.Themes)
	index.GeneratedAt = time.Now().UTC().Format(time.RFC3339)

	if dryRun {
		fmt.Printf("would update %s (generatedAt=%s)\n", indexPath, index.GeneratedAt)
		return merged, nil
	}

	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return merged, err
	}
	data = append(data, '\n')
	if err := os.WriteFile(indexPath, data, 0o644); err != nil {
		return merged, err
	}
	return merged, nil
}

func loadIndex(path string) (registryIndex, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return registryIndex{}, err
	}
	var index registryIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return registryIndex{}, fmt.Errorf("parse index: %w", err)
	}
	if index.Plugins == nil {
		index.Plugins = []registryEntry{}
	}
	if index.Themes == nil {
		index.Themes = []registryEntry{}
	}
	return index, nil
}

func loadEntry(path string) (registryEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return registryEntry{}, err
	}
	var entry registryEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return registryEntry{}, fmt.Errorf("parse entry: %w", err)
	}
	if entry.ID == "" || entry.Version == "" {
		return registryEntry{}, fmt.Errorf("missing id or version")
	}
	return entry, nil
}

func upsertEntry(entries *[]registryEntry, kind string, entry registryEntry) error {
	key := entryKey(entry)
	for _, existing := range *entries {
		if entryKey(existing) == key {
			return fmt.Errorf("duplicate %s entry %s", kind, key)
		}
	}
	*entries = append(*entries, entry)
	return nil
}

func entryKey(entry registryEntry) string {
	return entry.ID + "@" + entry.Version
}

func sortEntries(entries []registryEntry) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].ID == entries[j].ID {
			return entries[i].Version < entries[j].Version
		}
		return entries[i].ID < entries[j].ID
	})
}
