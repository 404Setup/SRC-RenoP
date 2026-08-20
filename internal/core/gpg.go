/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package core

import "errors"

var ErrGPGKeyLimit = errors.New("GPG key limit reached")

const (
	GPGReleaseQueued     = "queued"
	GPGReleaseValidating = "validating"
	GPGReleaseFailed     = "failed"
	GPGReleaseSuccess    = "success"
)

type GPGPublicKey struct {
	Fingerprint     string
	KeyID           string
	PrimaryIdentity string
	PublicKey       []byte
	KeyCreatedAt    int64
	KeyExpiresAt    int64
	FetchedAt       int64
}

type UserGPGKey struct {
	GPGPublicKey
	RequestedID string
	AddedAt     int64
}

type GPGSignature struct {
	ArtifactKey        string
	Repository         string
	ArtifactPath       string
	Fingerprint        string
	KeyID              string
	PrimaryIdentity    string
	Uploader           string
	SignatureCreatedAt int64
	VerifiedAt         int64
	HashAlgorithm      string
	PublicKeyAlgorithm string
}

// GPGRelease is a durable publication job for a Maven artifact protected by
// the GPG upload workflow. Staging paths are server-controlled quarantine
// files and must never be exposed by API conversion helpers.
type GPGRelease struct {
	ID                         string
	ActiveKey                  string
	Repository                 string
	ArtifactPath               string
	Uploader                   string
	Status                     string
	FailureReason              string
	ArtifactStagingPath        string
	SignatureStagingPath       string
	ArtifactMD5                string
	ArtifactSHA1               string
	ArtifactSHA256             string
	ArtifactSHA512             string
	SignatureMD5               string
	SignatureSHA1              string
	SignatureSHA256            string
	SignatureSHA512            string
	ArtifactSize               int64
	ArtifactModTime            int64
	SignatureSize              int64
	SignatureModTime           int64
	CreatedAt                  int64
	UpdatedAt                  int64
	CompletedAt                int64
	RequireSignature           bool
	ArtifactExisted            bool
	SignatureExisted           bool
	ArtifactGenerateChecksums  bool
	SignatureGenerateChecksums bool
	PublishStarted             bool
	CleanupPending             bool
}
