/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package main

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/molecule-man/go-brrr"

	"renop/internal/testutil"
)

func decodeSidecar(t *testing.T, path, encoding string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var reader io.Reader
	closeReader := func() error { return nil }
	switch encoding {
	case "deflate":
		var closer io.ReadCloser
		closer, err = zlib.NewReader(bytes.NewReader(data))
		reader, closeReader = closer, closer.Close
	case "gzip":
		var closer io.ReadCloser
		closer, err = gzip.NewReader(bytes.NewReader(data))
		reader, closeReader = closer, closer.Close
	case "zstd":
		var decoder *zstd.Decoder
		decoder, err = zstd.NewReader(bytes.NewReader(data))
		reader = decoder
		closeReader = func() error {
			decoder.Close()
			return nil
		}
	case "br":
		reader = brrr.NewReader(bytes.NewReader(data))
	}
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := io.ReadAll(reader)
	closeErr := closeReader()
	if err != nil {
		t.Fatal(err)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	return decoded
}

func TestPrecompressCreatesFourBalancedSidecarsWithoutRecursion(t *testing.T) {
	if balancedBrotliQuality != 9 {
		t.Fatalf("Brotli quality = %d, want 9", balancedBrotliQuality)
	}
	root := testutil.TempDir(t)
	source := filepath.Join(root, "app.js")
	want := bytes.Repeat([]byte("const renop = 'precompressed';\n"), 2048)
	if err := os.WriteFile(source, want, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "already.js.br"), []byte("existing sidecar"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "image.png"), want, 0o644); err != nil {
		t.Fatal(err)
	}

	for range 2 {
		variants, err := precompress(root)
		if err != nil {
			t.Fatal(err)
		}
		if variants != len(assetFormats) {
			t.Fatalf("variants = %d, want %d", variants, len(assetFormats))
		}
	}
	for _, format := range assetFormats {
		if got := decodeSidecar(t, source+format.suffix, format.name); !bytes.Equal(got, want) {
			t.Fatalf("%s round trip changed the asset", format.name)
		}
		if _, err := os.Stat(source + format.suffix + format.suffix); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s sidecar was compressed recursively", format.name)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "image.png.br")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("already-compressed image received a sidecar")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3+len(assetFormats) {
		t.Fatalf("asset count after repeated precompression = %d, want %d", len(entries), 3+len(assetFormats))
	}
}
