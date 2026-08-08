/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package updater

import (
	"archive/zip"
	"context"
	"debug/elf"
	"encoding/binary"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDownloadAndExtractValidatesURLAndDigestBeforeNetwork(t *testing.T) {
	validDigest := strings.Repeat("0", 64)
	tests := []struct {
		name      string
		url       string
		digest    string
		wantError string
	}{
		{name: "plain HTTP", url: "http://updates.example/renop.zip", digest: validDigest, wantError: "HTTPS URL"},
		{name: "URL user info", url: "https://user@updates.example/renop.zip", digest: validDigest, wantError: "without user info"},
		{name: "missing digest", url: "https://updates.example/renop.zip", digest: "", wantError: "valid SHA-256"},
		{name: "malformed digest", url: "https://updates.example/renop.zip", digest: "not-a-digest", wantError: "valid SHA-256"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DownloadAndExtract(context.Background(), tc.url, tc.digest)
			if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("error = %v, want text %q", err, tc.wantError)
			}
		})
	}
}

func TestValidateExecutableBinary_CurrentExe(t *testing.T) {
	exePath, err := os.Executable()
	if err != nil {
		t.Skip("Cannot resolve current executable path")
	}
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		t.Skip("Cannot eval symlinks for current executable path")
	}

	err = ValidateExecutableBinary(exePath)
	if err != nil {
		t.Fatalf("ValidateExecutableBinary failed on current executable: %v", err)
	}
}

func TestValidateExecutableBinary_InvalidFile(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "renop-dummy-*.exe")
	if err != nil {
		t.Fatalf("Failed to create dummy file: %v", err)
	}
	defer func() {
		_ = tmpFile.Close()
		_ = os.Remove(tmpFile.Name())
	}()

	_, _ = tmpFile.WriteString("not a binary file content")
	_ = tmpFile.Close()

	err = ValidateExecutableBinary(tmpFile.Name())
	if err == nil {
		t.Fatal("Expected error when validating non-binary file, got nil")
	}
}

// writeMinimalELF64 writes a 64-byte ELF header with the given machine and data encoding.
func writeMinimalELF64(t *testing.T, machine elf.Machine, data elf.Data) string {
	t.Helper()
	buf := make([]byte, 64)
	buf[0] = 0x7f
	buf[1] = 'E'
	buf[2] = 'L'
	buf[3] = 'F'
	buf[4] = byte(elf.ELFCLASS64)
	buf[5] = byte(data)
	buf[6] = byte(elf.EV_CURRENT)
	var order binary.ByteOrder = binary.LittleEndian
	if data == elf.ELFDATA2MSB {
		order = binary.BigEndian
	}
	order.PutUint16(buf[16:18], uint16(elf.ET_EXEC))
	order.PutUint16(buf[18:20], uint16(machine))
	order.PutUint32(buf[20:24], uint32(elf.EV_CURRENT))

	tmp, err := os.CreateTemp("", "renop-elf-*")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	path := tmp.Name()
	if _, err := tmp.Write(buf); err != nil {
		_ = tmp.Close()
		_ = os.Remove(path)
		t.Fatalf("Write: %v", err)
	}
	_ = tmp.Close()
	t.Cleanup(func() { _ = os.Remove(path) })
	return path
}

func assertELF(t *testing.T, goarch string, machine elf.Machine, data elf.Data, wantErr bool) {
	t.Helper()
	path := writeMinimalELF64(t, machine, data)
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	err = validateELF(f, "linux", goarch)
	if wantErr && err == nil {
		t.Fatalf("goarch=%s machine=%s: expected error, got nil", goarch, machine)
	}
	if !wantErr && err != nil {
		t.Fatalf("goarch=%s machine=%s: unexpected error: %v", goarch, machine, err)
	}
}

func TestValidateELF_KnownArchMatch(t *testing.T) {
	assertELF(t, "amd64", elf.EM_X86_64, elf.ELFDATA2LSB, false)
	assertELF(t, "arm64", elf.EM_AARCH64, elf.ELFDATA2LSB, false)
	assertELF(t, "riscv64", elf.EM_RISCV, elf.ELFDATA2LSB, false)
	assertELF(t, "loong64", elf.EM_LOONGARCH, elf.ELFDATA2LSB, false)
}

func TestValidateELF_KnownArchMismatch(t *testing.T) {
	assertELF(t, "loong64", elf.EM_X86_64, elf.ELFDATA2LSB, true)
	assertELF(t, "amd64", elf.EM_AARCH64, elf.ELFDATA2LSB, true)
}

func TestValidateELF_UnknownArchAccepted(t *testing.T) {
	assertELF(t, "ppc64le", elf.EM_PPC64, elf.ELFDATA2LSB, false)
	assertELF(t, "s390x", elf.EM_S390, elf.ELFDATA2MSB, false)
	assertELF(t, "loong64", elf.EM_LOONGARCH, elf.ELFDATA2LSB, false)
}

func TestValidateELF_AnyUnixOSLabel(t *testing.T) {
	path := writeMinimalELF64(t, elf.EM_X86_64, elf.ELFDATA2LSB)
	for _, goos := range []string{"linux", "freebsd", "netbsd", "openbsd", "dragonfly", "solaris"} {
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		err = validateELF(f, goos, "amd64")
		_ = f.Close()
		if err != nil {
			t.Fatalf("validateELF(%s/amd64): %v", goos, err)
		}
	}
}

func TestArchiveMatchesPlatform(t *testing.T) {
	assertArchiveMatch(t, "renop-abc-linux-amd64.zip", "linux", "amd64", true)
	assertArchiveMatch(t, "renop-abc-linux-arm64.zip", "linux", "amd64", false)
	assertArchiveMatch(t, "renop-abc-windows-amd64.zip", "windows", "amd64", true)
	assertArchiveMatch(t, "renop-abc-windows-arm64.zip", "windows", "amd64", false)
	assertArchiveMatch(t, "renop-abc-linux-loong64.zip", "linux", "loong64", true)
	assertArchiveMatch(t, "artifacts/renop-1-freebsd-arm64.zip", "freebsd", "arm64", true)
	assertArchiveMatch(t, "renop-1-dragonfly-amd64.zip", "dragonfly", "amd64", true)
	assertArchiveMatch(t, "not-a-platform.zip", "linux", "amd64", false)
}

func assertArchiveMatch(t *testing.T, name, goos, goarch string, want bool) {
	t.Helper()
	got := archiveMatchesPlatform(name, goos, goarch)
	if got != want {
		t.Errorf("archiveMatchesPlatform(%q, %s, %s)=%v, want %v", name, goos, goarch, got, want)
	}
}

func writeZip(t *testing.T, entries map[string][]byte) string {
	t.Helper()
	tmp, err := os.CreateTemp("", "renop-pkg-*.zip")
	if err != nil {
		t.Fatal(err)
	}
	path := tmp.Name()
	zw := zip.NewWriter(tmp)
	for name, body := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := tmp.Close(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(path) })
	return path
}

func zipBytes(t *testing.T, entries map[string][]byte) []byte {
	t.Helper()
	path := writeZip(t, entries)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestExtractExecutableFromZip_FlatPackage(t *testing.T) {
	exePath, err := os.Executable()
	if err != nil {
		t.Skip(err)
	}
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		t.Skip(err)
	}
	body, err := os.ReadFile(exePath)
	if err != nil {
		t.Fatal(err)
	}

	exeName := "renop"
	if runtime.GOOS == "windows" {
		exeName = "renop.exe"
	}
	pkg := writeZip(t, map[string][]byte{
		exeName:     body,
		"LICENSE":   []byte("license"),
		"README.md": []byte("readme"),
	})

	f, err := os.Open(pkg)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	out, err := ExtractExecutableFromZip(f)
	if err != nil {
		t.Fatalf("ExtractExecutableFromZip flat: %v", err)
	}
	defer os.Remove(out)

	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(body) {
		t.Fatalf("extracted size %d, want %d", len(got), len(body))
	}
}

func TestExtractExecutableFromZip_NightlyNested(t *testing.T) {
	exePath, err := os.Executable()
	if err != nil {
		t.Skip(err)
	}
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		t.Skip(err)
	}
	body, err := os.ReadFile(exePath)
	if err != nil {
		t.Fatal(err)
	}

	exeName := "renop"
	if runtime.GOOS == "windows" {
		exeName = "renop.exe"
	}

	wrongInner := zipBytes(t, map[string][]byte{
		exeName: []byte("not-a-real-binary-for-wrong-platform"),
	})
	rightInner := zipBytes(t, map[string][]byte{
		exeName:     body,
		"LICENSE":   []byte("x"),
		"README.md": []byte("y"),
	})

	wrongName := "renop-aaa-linux-loong64.zip"
	if runtime.GOOS == "linux" && runtime.GOARCH == "loong64" {
		wrongName = "renop-aaa-windows-amd64.zip"
	}
	rightName := "renop-nightly-" + runtime.GOOS + "-" + runtime.GOARCH + ".zip"

	outer := writeZip(t, map[string][]byte{
		wrongName:                         wrongInner,
		rightName:                         rightInner,
		"renop-nightly-openbsd-arm64.zip": wrongInner,
	})

	f, err := os.Open(outer)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	out, err := ExtractExecutableFromZip(f)
	if err != nil {
		t.Fatalf("ExtractExecutableFromZip nested nightly: %v", err)
	}
	defer os.Remove(out)

	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(body) {
		t.Fatalf("picked wrong inner package: size %d want %d", len(got), len(body))
	}
}

func TestExtractExecutableFromZip_NightlyMissingPlatform(t *testing.T) {
	junk := zipBytes(t, map[string][]byte{
		"renop": []byte("junk"),
	})
	outer := writeZip(t, map[string][]byte{
		"renop-nightly-openbsd-loong64.zip": junk,
		"renop-nightly-netbsd-riscv64.zip":  junk,
	})
	if archiveMatchesPlatform("renop-nightly-openbsd-loong64.zip", runtime.GOOS, runtime.GOARCH) ||
		archiveMatchesPlatform("renop-nightly-netbsd-riscv64.zip", runtime.GOOS, runtime.GOARCH) {
		t.Skip("host matches fixture platform")
	}

	f, err := os.Open(outer)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	out, err := ExtractExecutableFromZip(f)
	if err == nil {
		_ = os.Remove(out)
		t.Fatal("expected error when platform package is missing")
	}
}
