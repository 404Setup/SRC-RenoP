/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

// Command renop-precompress creates balanced HTTP content-coding sidecars for built frontend assets.
package main

import (
	"compress/gzip"
	"compress/zlib"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/klauspost/compress/zstd"
	"github.com/molecule-man/go-brrr"
)

const (
	copyBufferSize           = 128 << 10
	balancedDeflateLevel     = 6
	balancedGzipLevel        = 6
	balancedBrotliQuality    = 9
	precompressionTempPrefix = ".renop-precompress-"
)

type assetFormat struct {
	name   string
	suffix string
}

var assetFormats = [...]assetFormat{
	{name: "deflate", suffix: ".deflate"},
	{name: "gzip", suffix: ".gz"},
	{name: "zstd", suffix: ".zst"},
	{name: "br", suffix: ".br"},
}

var compressibleExtensions = map[string]struct{}{
	".css": {}, ".html": {}, ".js": {}, ".json": {}, ".map": {},
	".svg": {}, ".txt": {}, ".wasm": {}, ".xml": {},
}

func isPrecompressedPath(path string) bool {
	lower := strings.ToLower(path)
	for _, format := range assetFormats {
		if strings.HasSuffix(lower, format.suffix) {
			return true
		}
	}
	return false
}

func isCompressibleAsset(path string) bool {
	if strings.HasPrefix(filepath.Base(path), precompressionTempPrefix) || isPrecompressedPath(path) {
		return false
	}
	_, ok := compressibleExtensions[strings.ToLower(filepath.Ext(path))]
	return ok
}

func newAssetCompressor(format assetFormat, output io.Writer, inputSize int64) (io.WriteCloser, error) {
	switch format.name {
	case "deflate":
		return zlib.NewWriterLevel(output, balancedDeflateLevel)
	case "gzip":
		return gzip.NewWriterLevel(output, balancedGzipLevel)
	case "zstd":
		return zstd.NewWriter(output,
			zstd.WithEncoderLevel(zstd.SpeedDefault),
			zstd.WithEncoderConcurrency(1))
	case "br":
		sizeHint := uint(0)
		if inputSize >= 0 && uint64(inputSize) <= uint64(^uint(0)) {
			sizeHint = uint(inputSize)
		}
		return brrr.NewWriterOptions(output, balancedBrotliQuality, brrr.WriterOptions{SizeHint: sizeHint})
	default:
		return nil, fmt.Errorf("unsupported asset format %q", format.name)
	}
}

func compressAsset(inputPath string, format assetFormat, buffer []byte) (returnErr error) {
	input, err := os.Open(inputPath)
	if err != nil {
		return fmt.Errorf("open %s: %w", inputPath, err)
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil {
		return fmt.Errorf("inspect %s: %w", inputPath, err)
	}
	if !info.Mode().IsRegular() {
		return nil
	}

	outputPath := inputPath + format.suffix
	temporary, err := os.CreateTemp(filepath.Dir(outputPath), precompressionTempPrefix)
	if err != nil {
		return fmt.Errorf("create %s sidecar: %w", format.name, err)
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = temporary.Close()
		if returnErr != nil {
			_ = os.Remove(temporaryPath)
		}
	}()

	writer, err := newAssetCompressor(format, temporary, info.Size())
	if err != nil {
		return err
	}
	_, copyErr := io.CopyBuffer(writer, input, buffer)
	closeWriterErr := writer.Close()
	closeFileErr := temporary.Close()
	if copyErr != nil {
		return fmt.Errorf("compress %s with %s: %w", inputPath, format.name, copyErr)
	}
	if closeWriterErr != nil {
		return fmt.Errorf("finalize %s sidecar: %w", format.name, closeWriterErr)
	}
	if closeFileErr != nil {
		return fmt.Errorf("close %s sidecar: %w", format.name, closeFileErr)
	}
	if err := os.Chmod(temporaryPath, 0o644); err != nil {
		return fmt.Errorf("set %s sidecar permissions: %w", format.name, err)
	}
	if err := os.Remove(outputPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("replace %s sidecar: %w", format.name, err)
	}
	if err := os.Rename(temporaryPath, outputPath); err != nil {
		return fmt.Errorf("publish %s sidecar: %w", format.name, err)
	}
	return nil
}

func precompress(root string) (int, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return 0, err
	}
	info, err := os.Stat(root)
	if err != nil {
		return 0, fmt.Errorf("inspect asset root: %w", err)
	}
	if !info.IsDir() {
		return 0, errors.New("asset root must be a directory")
	}

	sources := make([]string, 0, 32)
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type().IsRegular() && isCompressibleAsset(path) {
			sources = append(sources, path)
		}
		return nil
	}); err != nil {
		return 0, fmt.Errorf("scan frontend assets: %w", err)
	}
	sort.Strings(sources)
	buffer := make([]byte, copyBufferSize)
	variants := 0
	for _, source := range sources {
		for _, format := range assetFormats {
			if err := compressAsset(source, format, buffer); err != nil {
				return variants, err
			}
			variants++
		}
	}
	return variants, nil
}

func run(args []string) error {
	flags := flag.NewFlagSet("renop-precompress", flag.ContinueOnError)
	root := flags.String("root", "", "built frontend asset directory")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*root) == "" {
		return errors.New("-root is required")
	}
	variants, err := precompress(*root)
	if err != nil {
		return err
	}
	fmt.Printf("Precompressed %d frontend variants\n", variants)
	return nil
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "renop-precompress:", err)
		os.Exit(1)
	}
}
