/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the terms of the Mozilla Public License, v. 2.0.
 */

package cargo

import (
	"errors"
	"strings"

	"golang.org/x/mod/semver"
)

func validatePackage(name, version string) error {
	if err := validateCrateName(name); err != nil {
		return err
	}
	if len(version) == 0 || len(version) > 128 || !semver.IsValid("v"+version) {
		return errors.New("invalid Cargo crate version")
	}
	return nil
}

func validateCrateName(name string) error {
	if len(name) == 0 || len(name) > 64 || !isASCIIAlpha(name[0]) {
		return errors.New("invalid Cargo crate name")
	}
	for i := 1; i < len(name); i++ {
		char := name[i]
		if isASCIIAlpha(char) || (char >= '0' && char <= '9') || char == '-' || char == '_' {
			continue
		}
		return errors.New("invalid Cargo crate name")
	}
	lower := strings.ToLower(name)
	if lower == "con" || lower == "prn" || lower == "aux" || lower == "nul" ||
		(len(lower) == 4 && (strings.HasPrefix(lower, "com") || strings.HasPrefix(lower, "lpt")) &&
			lower[3] >= '1' && lower[3] <= '9') {
		return errors.New("invalid Cargo crate name")
	}
	return nil
}

func isASCIIAlpha(char byte) bool {
	return (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z')
}

func sameVersion(left, right string) bool {
	leftSemver := "v" + left
	rightSemver := "v" + right
	if !semver.IsValid(leftSemver) || !semver.IsValid(rightSemver) {
		return left == right
	}
	return semver.Compare(leftSemver, rightSemver) == 0
}

func indexDependencies(deps []PublishDependency) ([]IndexDependency, error) {
	if deps == nil {
		return []IndexDependency{}, nil
	}
	result := make([]IndexDependency, 0, len(deps))
	for _, dep := range deps {
		if err := validateCrateName(dep.Name); err != nil {
			return nil, errors.New("invalid Cargo dependency name")
		}
		if !validMetadataText(dep.VersionReq, 1024, false) {
			return nil, errors.New("invalid Cargo dependency requirement")
		}
		name := dep.Name
		var packageName *string
		if dep.ExplicitNameInToml != nil {
			alias := strings.TrimSpace(*dep.ExplicitNameInToml)
			if err := validateCrateName(alias); err != nil {
				return nil, errors.New("invalid Cargo dependency alias")
			}
			name = alias
			original := dep.Name
			packageName = &original
		}
		kind := strings.ToLower(strings.TrimSpace(dep.Kind))
		if kind == "" {
			kind = "normal"
		}
		if kind != "normal" && kind != "dev" && kind != "build" {
			return nil, errors.New("invalid Cargo dependency kind")
		}
		if !validOptionalMetadataText(dep.Target, 4096) || !validOptionalMetadataText(dep.Registry, 4096) {
			return nil, errors.New("invalid Cargo dependency metadata")
		}
		features := dep.Features
		if features == nil {
			features = []string{}
		}
		for _, feature := range features {
			if !validMetadataText(feature, 1024, true) {
				return nil, errors.New("invalid Cargo dependency feature")
			}
		}
		defaultFeatures := true
		if dep.DefaultFeatures != nil {
			defaultFeatures = *dep.DefaultFeatures
		}
		result = append(result, IndexDependency{
			Name: name, Requirement: dep.VersionReq, Features: features,
			Optional: dep.Optional, DefaultFeatures: defaultFeatures,
			Target: dep.Target, Kind: kind, Registry: dep.Registry, Package: packageName,
		})
	}
	return result, nil
}

func validatePublishMetadata(metadata *PublishMetadata) error {
	if metadata == nil {
		return errors.New("invalid Cargo publish metadata")
	}
	if !validOptionalMetadataText(metadata.Links, 1024) || !validOptionalMetadataText(metadata.RustVersion, 128) ||
		!validMetadataText(metadata.Description, 4000, true) {
		return errors.New("invalid Cargo publish metadata")
	}
	if err := validateFeatureMap(metadata.Features); err != nil {
		return err
	}
	return validateFeatureMap(metadata.Features2)
}

func validateFeatureMap(features map[string][]string) error {
	for name, members := range features {
		if !validMetadataText(name, 1024, true) {
			return errors.New("invalid Cargo feature name")
		}
		for _, member := range members {
			if !validMetadataText(member, 1024, true) {
				return errors.New("invalid Cargo feature member")
			}
		}
	}
	return nil
}

func normalizeCrateName(name string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(name)), "_", "-")
}

func validOptionalMetadataText(value *string, max int) bool {
	return value == nil || validMetadataText(*value, max, true)
}

func validMetadataText(value string, max int, allowEmpty bool) bool {
	if (!allowEmpty && value == "") || len(value) > max {
		return false
	}
	return !strings.ContainsAny(value, "\x00\r\n")
}
