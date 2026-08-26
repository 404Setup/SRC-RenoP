/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	brrr "github.com/molecule-man/go-brrr"
)

func TestCompressFileProducesRawBrotliStream(t *testing.T) {
	directory := t.TempDir()
	inputPath := filepath.Join(directory, "renop")
	outputPath := filepath.Join(directory, "renop.br")
	want := bytes.Repeat([]byte("RenoP update payload\n"), 4096)
	if err := os.WriteFile(inputPath, want, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := compressFile(inputPath, outputPath, brrr.BestCompression); err != nil {
		t.Fatal(err)
	}
	compressed, err := os.Open(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	decompressed, err := io.ReadAll(brrr.NewReader(compressed))
	closeErr := compressed.Close()
	if err != nil {
		t.Fatal(err)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	if !bytes.Equal(decompressed, want) {
		t.Fatal("Brotli round trip changed the executable bytes")
	}
}
