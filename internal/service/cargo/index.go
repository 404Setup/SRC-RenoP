/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package cargo

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"strings"

	"github.com/goccy/go-json"
)

const (
	maxIndexSize     = 16 << 20
	maxIndexLineSize = 2 << 20
)

var (
	errIndexTooLarge = errors.New("Cargo index exceeds the size limit")
	errVersionExists = errors.New("Crate version already exists")
)

func indexPath(crateName string) string {
	name := bytes.ToLower([]byte(crateName))
	switch len(name) {
	case 1:
		return "1/" + string(name)
	case 2:
		return "2/" + string(name)
	case 3:
		return "3/" + string(name[:1]) + "/" + string(name)
	default:
		return string(name[:2]) + "/" + string(name[2:4]) + "/" + string(name)
	}
}

type cargoIndexValidationEntry struct {
	Name    string `json:"name"`
	Version string `json:"vers"`
}

type limitedIndexWriter struct {
	writer io.Writer
	wrote  int64
}

func (writer *limitedIndexWriter) Write(data []byte) (int, error) {
	if writer.wrote+int64(len(data)) > maxIndexSize {
		return 0, errIndexTooLarge
	}
	n, err := writer.writer.Write(data)
	writer.wrote += int64(n)
	return n, err
}

func rewriteIndex(reader io.Reader, destination io.Writer, entry IndexEntry) error {
	limitedWriter := &limitedIndexWriter{writer: destination}
	versionExists := false
	err := scanIndex(reader, func(line []byte) error {
		var current cargoIndexValidationEntry
		if json.Unmarshal(line, &current) != nil || current.Name == "" || current.Version == "" {
			return errors.New("invalid Cargo index entry")
		}
		if !strings.EqualFold(current.Name, entry.Name) {
			return errors.New("Cargo crate name collides with an existing package")
		}
		if sameVersion(current.Version, entry.Version) {
			versionExists = true
		}
		if _, err := limitedWriter.Write(line); err != nil {
			return err
		}
		_, err := limitedWriter.Write([]byte{'\n'})
		return err
	})
	if err != nil {
		return err
	}
	if versionExists {
		return errVersionExists
	}
	return json.NewEncoder(limitedWriter).Encode(entry)
}

func rewriteYanked(reader io.Reader, destination io.Writer, version string, yanked bool) (bool, error) {
	limitedWriter := &limitedIndexWriter{writer: destination}
	versionFound := false
	err := scanIndex(reader, func(line []byte) error {
		if versionFound {
			if _, err := limitedWriter.Write(line); err != nil {
				return err
			}
			_, err := limitedWriter.Write([]byte{'\n'})
			return err
		}
		var entry IndexEntry
		if json.Unmarshal(line, &entry) != nil || entry.Version == "" {
			return errors.New("invalid Cargo index entry")
		}
		if entry.Version != version {
			if _, err := limitedWriter.Write(line); err != nil {
				return err
			}
			_, err := limitedWriter.Write([]byte{'\n'})
			return err
		}
		entry.Yanked = yanked
		versionFound = true
		return json.NewEncoder(limitedWriter).Encode(entry)
	})
	return versionFound, err
}

func rewriteRemoveVersion(reader io.Reader, destination io.Writer, version string) (removed bool, remaining int, err error) {
	limitedWriter := &limitedIndexWriter{writer: destination}
	err = scanIndex(reader, func(line []byte) error {
		var entry IndexEntry
		if json.Unmarshal(line, &entry) != nil || entry.Version == "" {
			return errors.New("invalid Cargo index entry")
		}
		if entry.Version == version {
			removed = true
			return nil
		}
		remaining++
		if _, err := limitedWriter.Write(line); err != nil {
			return err
		}
		_, err := limitedWriter.Write([]byte{'\n'})
		return err
	})
	return removed, remaining, err
}

func rewritePackageYanked(reader io.Reader, destination io.Writer, desired map[string]bool) (int, error) {
	limitedWriter := &limitedIndexWriter{writer: destination}
	updated := 0
	err := scanIndex(reader, func(line []byte) error {
		var entry IndexEntry
		if json.Unmarshal(line, &entry) != nil || entry.Version == "" {
			return errors.New("invalid Cargo index entry")
		}
		yanked, ok := desired[entry.Version]
		if !ok {
			return errors.New("Cargo index contains an unmanaged package version")
		}
		entry.Yanked = yanked
		updated++
		return json.NewEncoder(limitedWriter).Encode(entry)
	})
	return updated, err
}

func scanIndex(reader io.Reader, visit func([]byte) error) error {
	if reader == nil {
		return nil
	}
	scanner := bufio.NewScanner(io.LimitReader(reader, maxIndexSize+1))
	scanner.Buffer(make([]byte, 64<<10), maxIndexLineSize)
	var total int64
	for scanner.Scan() {
		line := scanner.Bytes()
		total += int64(len(line)) + 1
		if total > maxIndexSize {
			return errIndexTooLarge
		}
		if len(bytes.TrimSpace(line)) == 0 {
			return errors.New("invalid Cargo index entry")
		}
		if err := visit(line); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			return errIndexTooLarge
		}
		return err
	}
	return nil
}
