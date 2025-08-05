// File: file_utils.go
package utils

import (
	"encoding/csv"
	"fmt"
	"harbor-cleaner/internal/k8s"
	"os"
	"strings"
)

// ImageContext holds usage details for an image.
type ImageContext struct {
	Env       string
	Namespace string
}

// writeManifestToCSV writes the collected safe image info to a CSV manifest file.
func WriteManifestToCSV(records []k8s.SafeImageInfo, path string) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create manifest file: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Write header
	if err := writer.Write([]string{"image", "environment", "namespace"}); err != nil {
		return fmt.Errorf("failed to write header to manifest: %w", err)
	}

	// Write records
	for _, record := range records {
		if err := writer.Write([]string{record.Image, record.Env, record.Namespace}); err != nil {
			return fmt.Errorf("failed to write record to manifest: %w", err)
		}
	}
	return nil
}

// ReadManifestFromCSV reads the manifest file and returns both a simple safe list map
// and a map for looking up context.
func ReadManifestFromCSV(path string) (map[string]struct{}, map[string][]ImageContext, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open manifest file: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read manifest csv: %w", err)
	}

	safeImageSet := make(map[string]struct{})
	contextMap := make(map[string][]ImageContext)

	// Skip header row
	for i, record := range records {
		if i == 0 {
			continue
		}
		if len(record) >= 3 {
			image := strings.TrimSpace(record[0])
			env := strings.TrimSpace(record[1])
			ns := strings.TrimSpace(record[2])
			if image != "" {
				safeImageSet[image] = struct{}{}
				contextMap[image] = append(contextMap[image], ImageContext{Env: env, Namespace: ns})
			}
		}
	}
	return safeImageSet, contextMap, nil
}

// WriteAuditReport writes the final audit data to a CSV file.
func WriteAuditReport(records [][]string, path string) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create audit report file: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	return writer.WriteAll(records)
}

// ParseWhitelist parses a comma-separated string into a map for quick lookups.
func ParseWhitelist(whitelistCSV string) map[string]struct{} {
	if whitelistCSV == "" {
		return nil // Return nil to signify no whitelist is active
	}
	items := strings.Split(whitelistCSV, ",")
	whitelist := make(map[string]struct{}, len(items))
	for _, item := range items {
		whitelist[strings.TrimSpace(item)] = struct{}{}
	}
	return whitelist
}