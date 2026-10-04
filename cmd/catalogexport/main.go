// Command catalogexport builds an independently licensed, source-checked subset.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"raid-website/internal/lootcatalog"
	"raid-website/internal/lootcatalog/sourceexport"
)

func main() {
	manifestPath := flag.String("manifest", "", "source-evidence association manifest JSON; not human approval")
	outputPath := flag.String("out", "", "output catalog JSON in an existing directory")
	flag.Parse()
	if err := run(*manifestPath, *outputPath); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(manifestPath, outputPath string) error {
	if manifestPath == "" || outputPath == "" {
		return fmt.Errorf("-manifest and -out are required; no automatic production fetch")
	}
	info, err := os.Stat(filepath.Dir(outputPath))
	if err != nil || !info.IsDir() {
		return fmt.Errorf("output parent must be an existing directory")
	}
	input, err := os.Open(manifestPath)
	if err != nil {
		return err
	}
	defer func() { _ = input.Close() }()
	var manifest sourceexport.Manifest
	decoder := json.NewDecoder(io.LimitReader(input, 4<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("manifest must contain exactly one JSON object")
	}
	dataset, license, err := sourceexport.Export(context.Background(), manifest)
	if err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(dataset, "", "  ")
	if err != nil {
		return err
	}
	// Exclusive creation prevents clobbering previously reviewed artifacts.
	write := func(path string, content []byte) error {
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if err != nil {
			return err
		}
		_, writeErr := file.Write(content)
		closeErr := file.Close()
		if writeErr != nil {
			return writeErr
		}
		return closeErr
	}
	if err := write(outputPath+".LICENSE", []byte(license)); err != nil {
		return err
	}
	if err := write(outputPath, append(encoded, '\n')); err != nil {
		return err
	}
	_, report := lootcatalog.Canonical(dataset)
	coverageJSON, err := json.MarshalIndent(struct {
		RawHash  string        `json:"raw_hash"`
		Partial  bool          `json:"partial"`
		Coverage []coverageRow `json:"coverage"`
	}{report.RawHash, true, coverage(dataset)}, "", "  ")
	if err != nil {
		return err
	}
	if err := write(outputPath+".coverage.json", append(coverageJSON, '\n')); err != nil {
		return err
	}
	fmt.Printf("%s\nsha256:%s\ninstances:%d bosses:%d items:%d drops:%d\n", outputPath, report.RawHash, len(dataset.Instances), len(dataset.Bosses), len(dataset.Items), len(dataset.Drops))
	return nil
}
