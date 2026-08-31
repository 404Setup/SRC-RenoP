/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

// Command renop-brotli creates a raw RFC 7932 Brotli stream for one RenoP executable.
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/molecule-man/go-brrr"
)

const copyBufferSize = 256 << 10

func compressFile(inputPath, outputPath string, quality int) (returnErr error) {
	if quality < brrr.BestSpeed || quality > brrr.BestCompression {
		return fmt.Errorf("brotli quality must be between %d and %d", brrr.BestSpeed, brrr.BestCompression)
	}
	inputPath, err := filepath.Abs(inputPath)
	if err != nil {
		return err
	}
	outputPath, err = filepath.Abs(outputPath)
	if err != nil {
		return err
	}
	if inputPath == outputPath {
		return errors.New("input and output paths must differ")
	}
	input, err := os.Open(inputPath)
	if err != nil {
		return fmt.Errorf("open input: %w", err)
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil {
		return fmt.Errorf("inspect input: %w", err)
	}
	if !info.Mode().IsRegular() {
		return errors.New("input must be a regular file")
	}
	outputDirectory := filepath.Dir(outputPath)
	temporary, err := os.CreateTemp(outputDirectory, ".renop-brotli-*")
	if err != nil {
		return fmt.Errorf("create temporary output: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = temporary.Close()
		if returnErr != nil {
			_ = os.Remove(temporaryPath)
		}
	}()
	buffered := bufio.NewWriterSize(temporary, copyBufferSize)
	sizeHint := uint(0)
	if uint64(info.Size()) <= uint64(^uint(0)) {
		sizeHint = uint(info.Size())
	}
	writer, err := brrr.NewWriterOptions(buffered, quality, brrr.WriterOptions{SizeHint: sizeHint})
	if err != nil {
		return fmt.Errorf("create Brotli writer: %w", err)
	}
	buffer := make([]byte, copyBufferSize)
	if _, err := io.CopyBuffer(writer, input, buffer); err != nil {
		_ = writer.Close()
		return fmt.Errorf("compress input: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("finalize Brotli stream: %w", err)
	}
	if err := buffered.Flush(); err != nil {
		return fmt.Errorf("flush Brotli output: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync Brotli output: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close Brotli output: %w", err)
	}
	if err := os.Remove(outputPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("replace Brotli output: %w", err)
	}
	if err := os.Rename(temporaryPath, outputPath); err != nil {
		return fmt.Errorf("publish Brotli output: %w", err)
	}
	return nil
}

func run() error {
	input := flag.String("input", "", "path to the executable to compress")
	output := flag.String("output", "", "path to the raw .br output")
	quality := flag.Int("quality", 9, "Brotli quality from 0 to 11")
	flag.Parse()
	if *input == "" || *output == "" {
		return errors.New("both -input and -output are required")
	}
	return compressFile(*input, *output, *quality)
}

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "renop-brotli:", err)
		os.Exit(1)
	}
}
