/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package utils

import (
	"testing"
)

func TestSanitizePathValid(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{input: "foo/bar", expected: "foo/bar"},
		{input: "/foo/bar", expected: "foo/bar"},
		{input: "foo/bar/", expected: "foo/bar"},
		{input: "/foo/bar/", expected: "foo/bar"},
		{input: "foo", expected: "foo"},
		{input: "a/b/c.txt", expected: "a/b/c.txt"},
		{input: "foo+bar", expected: "foo+bar"},
		{input: "com/example/mod/1.2.0+26.1/mod-1.2.0+26.1.jar", expected: "com/example/mod/1.2.0+26.1/mod-1.2.0+26.1.jar"},
		{input: "com/example/mod/1.2.0%2B26.1/mod-1.2.0%2B26.1.jar", expected: "com/example/mod/1.2.0+26.1/mod-1.2.0+26.1.jar"},
		{input: "com/example/mod/1.2.0%2b26.1", expected: "com/example/mod/1.2.0+26.1"},
		{input: "%E4%B8%AD%E6%96%87/%E4%BE%9D%E8%B5%96%20%F0%9F%9A%80.jar", expected: "中文/依赖 🚀.jar"},
		{input: "中文/构件-版本.jar", expected: "中文/构件-版本.jar"},
		{input: "日本語/ライブラリ.jar", expected: "日本語/ライブラリ.jar"},
		{input: "emoji/依赖 🚀.pom", expected: "emoji/依赖 🚀.pom"},
	}

	for _, tt := range tests {
		res, ok := SanitizePath(tt.input)
		if !ok {
			t.Errorf("SanitizePath(%q) = false, expected %q", tt.input, tt.expected)
		} else if res != tt.expected {
			t.Errorf("SanitizePath(%q) = %q, expected %q", tt.input, res, tt.expected)
		}
	}
}

func TestSanitizePathInvalid(t *testing.T) {
	tests := []string{
		"foo/../bar",
		"..",
		"../foo",
		"foo\\bar",
		"C:\\foo",
		"C:/foo",
		"foo:bar",
		"foo\x00bar",
		"foo\nbar",
		"trailing-space ",
		"trailing-dot.",
		"CON",
		"aux.txt",
		"NUL",
		"com1.jar",
		"LPT1",
		string([]byte{0xff, 0xfe}),
	}

	for _, input := range tests {
		res, ok := SanitizePath(input)
		if ok {
			t.Errorf("SanitizePath(%q) expected false, got %q", input, res)
		}
	}
}

func TestSanitizePathPercentEncodedInvalid(t *testing.T) {
	tests := []string{
		"foo/%2E%2E/bar",
		"%2E%2E",
		"foo%5Cbar",
		"foo%2Fbar",
		"foo%3Abar",
		"%FF",
		"foo%",
		"foo%2",
	}

	for _, input := range tests {
		res, ok := SanitizePath(input)
		if ok {
			t.Errorf("SanitizePath(%q) expected false, got %q", input, res)
		}
	}
}

func TestSanitizePathExtraEdgeCases(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{input: "", expected: ""},
		{input: ".", expected: ""},
		{input: "./foo", expected: "foo"},
		{input: "foo/./bar", expected: "foo/bar"},
		{input: "foo///bar", expected: "foo/bar"},
		{input: "///foo", expected: "foo"},
	}

	for _, tt := range tests {
		res, ok := SanitizePath(tt.input)
		if !ok {
			t.Errorf("SanitizePath(%q) = false, expected %q", tt.input, tt.expected)
		} else if res != tt.expected {
			t.Errorf("SanitizePath(%q) = %q, expected %q", tt.input, res, tt.expected)
		}
	}
}

func TestIsValidRepositoryName(t *testing.T) {
	for _, name := range []string{"releases", "private-repo", "仓库"} {
		if !IsValidRepositoryName(name) {
			t.Errorf("expected repository name %q to be valid", name)
		}
	}
	for _, name := range []string{"", ".", "..", "a/b", `a\b`, "bad:name", "trailing."} {
		if IsValidRepositoryName(name) {
			t.Errorf("expected repository name %q to be invalid", name)
		}
	}
}

func TestIsImageFileSupportedExtensions(t *testing.T) {
	tests := []string{
		"image.png",
		"image.jpg",
		"image.jpeg",
		"image.gif",
		"image.webp",
		"image.bmp",
		"image.svg",
		"image.ico",
	}

	for _, ext := range tests {
		if !IsImageFile(ext) {
			t.Errorf("Expected IsImageFile to return true for %q", ext)
		}
	}
}

func TestIsImageFileCaseInsensitive(t *testing.T) {
	tests := []string{
		"IMAGE.PNG",
		"image.JpG",
		"test.WebP",
		"icon.ICO",
	}

	for _, ext := range tests {
		if !IsImageFile(ext) {
			t.Errorf("Expected IsImageFile to return true for %q", ext)
		}
	}
}

func TestIsImageFileComplexPaths(t *testing.T) {
	tests := []string{
		"/path/to/my/image.png",
		"C:\\windows\\path\\image.bmp",
		"../relative/path/image.gif",
		"my.awesome.image.1.2.3.jpg",
		".hidden_image.png",
		"folder.with.dot/image.svg",
	}

	for _, p := range tests {
		if !IsImageFile(p) {
			t.Errorf("Expected IsImageFile to return true for %q", p)
		}
	}
}

func TestIsImageFileNegativeCases(t *testing.T) {
	tests := []string{
		"document.txt",
		"script.js",
		"archive.zip",
		"image.png.txt",
		"image.png_",
		"image.png ",
		"image.png/",
		"image.png\\",
	}

	for _, p := range tests {
		if IsImageFile(p) {
			t.Errorf("Expected IsImageFile to return false for %q", p)
		}
	}
}

func TestIsImageFileEdgeCases(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{input: "no_extension", expected: false},
		{input: "", expected: false},
		{input: ".", expected: false},
		{input: "just_a_dot.", expected: false},
		{input: "🚀.png", expected: true},
		{input: "日本語.jpg", expected: true},
		{input: "spaces in name.gif", expected: true},
		{input: ".png", expected: true},
	}

	for _, tt := range tests {
		if IsImageFile(tt.input) != tt.expected {
			t.Errorf("IsImageFile(%q) = %v, expected %v", tt.input, IsImageFile(tt.input), tt.expected)
		}
	}
}

func TestIsPreviewableTextFileIncludesGradleModuleMetadata(t *testing.T) {
	for _, path := range []string{"demo.module", "demo.MODULE", "group/demo.module"} {
		if !IsPreviewableTextFile(path) {
			t.Errorf("IsPreviewableTextFile(%q) = false, expected true", path)
		}
	}
}
