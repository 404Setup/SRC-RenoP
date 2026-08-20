/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package gpg

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rsa"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
	"github.com/ProtonMail/go-crypto/openpgp/packet"

	"renop/internal/core"
)

const MaxDetachedSignatureSize = 1024 * 1024

var (
	ErrSignatureInvalid       = errors.New("GPG signature is invalid")
	ErrSigningKeyUnregistered = errors.New("GPG signing key is not registered for the uploader")
)

type parsedDetachedSignature struct {
	Body              []byte
	IssuerKeyID       uint64
	IssuerFingerprint string
	CreationTime      time.Time
}

func parseDetachedSignature(armored []byte) (*parsedDetachedSignature, error) {
	if len(armored) == 0 || len(armored) > MaxDetachedSignatureSize {
		return nil, ErrSignatureInvalid
	}
	trimmed := bytes.TrimSpace(armored)
	beginMarker := []byte("-----BEGIN PGP SIGNATURE-----")
	endMarker := []byte("-----END PGP SIGNATURE-----")
	if !bytes.HasPrefix(trimmed, beginMarker) || !bytes.HasSuffix(trimmed, endMarker) ||
		bytes.Count(trimmed, beginMarker) != 1 || bytes.Count(trimmed, endMarker) != 1 {
		return nil, ErrSignatureInvalid
	}
	block, err := armor.Decode(bytes.NewReader(armored))
	if err != nil || block.Type != openpgp.SignatureType {
		return nil, ErrSignatureInvalid
	}
	limited := &io.LimitedReader{R: block.Body, N: MaxDetachedSignatureSize + 1}
	body, err := io.ReadAll(limited)
	if err != nil || limited.N <= 0 || len(body) == 0 {
		return nil, ErrSignatureInvalid
	}
	reader := packet.NewReader(bytes.NewReader(body))
	p, err := reader.Next()
	if err != nil {
		return nil, ErrSignatureInvalid
	}
	sig, ok := p.(*packet.Signature)
	if !ok || sig.IssuerKeyId == nil {
		return nil, ErrSignatureInvalid
	}
	if sig.SigType != packet.SigTypeBinary && sig.SigType != packet.SigTypeText {
		return nil, ErrSignatureInvalid
	}
	if sig.CreationTime.After(time.Now().Add(5 * time.Minute)) {
		return nil, ErrSignatureInvalid
	}
	if _, err := reader.Next(); err != io.EOF {
		return nil, ErrSignatureInvalid
	}
	return &parsedDetachedSignature{
		Body:              body,
		IssuerKeyID:       *sig.IssuerKeyId,
		IssuerFingerprint: strings.ToUpper(hex.EncodeToString(sig.IssuerFingerprint)),
		CreationTime:      sig.CreationTime,
	}, nil
}

func entityIssuerKey(entity *openpgp.Entity, signature *parsedDetachedSignature) *packet.PublicKey {
	if entity == nil || signature == nil {
		return nil
	}
	check := func(key *packet.PublicKey) bool {
		if key == nil || key.KeyId != signature.IssuerKeyID {
			return false
		}
		if signature.IssuerFingerprint == "" {
			return true
		}
		return strings.EqualFold(hex.EncodeToString(key.Fingerprint), signature.IssuerFingerprint)
	}
	if check(entity.PrimaryKey) {
		return entity.PrimaryKey
	}
	for i := range entity.Subkeys {
		if check(entity.Subkeys[i].PublicKey) {
			return entity.Subkeys[i].PublicKey
		}
	}
	return nil

}

func isAcceptedSigningKey(key *packet.PublicKey) bool {
	if key == nil {
		return false
	}
	switch key.PubKeyAlgo {
	case packet.PubKeyAlgoRSA, packet.PubKeyAlgoRSASignOnly:
		publicKey, ok := key.PublicKey.(*rsa.PublicKey)
		return ok && publicKey.N != nil && publicKey.N.BitLen() >= 2048
	case packet.PubKeyAlgoECDSA,
		packet.PubKeyAlgoEdDSA,
		packet.PubKeyAlgoEd25519,
		packet.PubKeyAlgoEd448:
		return true
	default:
		return false
	}
}

func entityHasAcceptedSigningKey(entity *openpgp.Entity, now time.Time) bool {
	if entity == nil || !isAcceptedSigningKey(entity.PrimaryKey) {
		return false
	}
	if key, ok := entity.SigningKeyById(now, entity.PrimaryKey.KeyId); ok && isAcceptedSigningKey(key.PublicKey) {
		return true
	}
	for i := range entity.Subkeys {
		publicKey := entity.Subkeys[i].PublicKey
		if publicKey == nil || !isAcceptedSigningKey(publicKey) {
			continue
		}
		if key, ok := entity.SigningKeyById(now, publicKey.KeyId); ok && isAcceptedSigningKey(key.PublicKey) {
			return true
		}
	}
	return false
}

func publicKeyAlgorithmName(algorithm packet.PublicKeyAlgorithm) string {
	switch algorithm {
	case packet.PubKeyAlgoRSA:
		return "RSA"
	case packet.PubKeyAlgoRSAEncryptOnly:
		return "RSA (encrypt only)"
	case packet.PubKeyAlgoRSASignOnly:
		return "RSA (sign only)"
	case packet.PubKeyAlgoDSA:
		return "DSA"
	case packet.PubKeyAlgoECDSA:
		return "ECDSA"
	case packet.PubKeyAlgoEdDSA:
		return "EdDSA"
	case packet.PubKeyAlgoEd25519:
		return "Ed25519"
	case packet.PubKeyAlgoEd448:
		return "Ed448"
	default:
		return fmt.Sprintf("OpenPGP algorithm %d", algorithm)
	}
}

func VerifyDetached(
	ctx context.Context,
	state *core.AppState,
	username string,
	artifact io.Reader,
	armoredSignature []byte,
	repository string,
	artifactPath string,
) (*core.GPGSignature, error) {
	if state == nil || state.Inner == nil || artifact == nil || username == "" {
		return nil, ErrSignatureInvalid
	}
	parsed, err := parseDetachedSignature(armoredSignature)
	if err != nil {
		return nil, err
	}
	db := state.GetDB()
	if db == nil {
		return nil, core.ErrDatabaseUnavailable
	}
	userKeys, err := db.ListUserGPGKeys(username)
	if err != nil {
		return nil, err
	}

	candidates := make(openpgp.EntityList, 0, len(userKeys))
	registeredIssuer := false
	for _, userKey := range userKeys {
		key := &userKey.GPGPublicKey
		if !isFresh(key, time.Now()) {
			key, err = refreshPublicKey(ctx, state, key.Fingerprint)
			if err != nil {
				return nil, fmt.Errorf("failed to refresh GPG signing key: %w", err)
			}
		}
		entities, parseErr := openpgp.ReadKeyRing(bytes.NewReader(key.PublicKey))
		if parseErr != nil || len(entities) != 1 {
			continue
		}
		issuerKey := entityIssuerKey(entities[0], parsed)
		if issuerKey == nil {
			continue
		}
		registeredIssuer = true
		if !isAcceptedSigningKey(issuerKey) {
			continue
		}
		candidates = append(candidates, entities[0])
	}
	if len(candidates) == 0 {
		if registeredIssuer {
			return nil, ErrSignatureInvalid
		}
		return nil, ErrSigningKeyUnregistered
	}

	sig, signer, err := openpgp.VerifyDetachedSignatureAndHash(
		candidates,
		artifact,
		bytes.NewReader(parsed.Body),
		[]crypto.Hash{crypto.SHA256, crypto.SHA384, crypto.SHA512},
		&packet.Config{},
	)
	if err != nil || sig == nil || signer == nil || signer.PrimaryKey == nil {
		return nil, ErrSignatureInvalid
	}
	identity := ""
	if primary := signer.PrimaryIdentity(); primary != nil {
		identity = primary.Name
	}
	return &core.GPGSignature{
		ArtifactKey:        ArtifactKey(repository, artifactPath),
		Repository:         repository,
		ArtifactPath:       strings.TrimPrefix(filepathSlashClean(artifactPath), "/"),
		Fingerprint:        strings.ToUpper(hex.EncodeToString(signer.PrimaryKey.Fingerprint)),
		KeyID:              fmt.Sprintf("%016X", parsed.IssuerKeyID),
		PrimaryIdentity:    identity,
		Uploader:           strings.ToLower(username),
		SignatureCreatedAt: sig.CreationTime.UnixMilli(),
		VerifiedAt:         time.Now().UnixMilli(),
		HashAlgorithm:      sig.Hash.String(),
		PublicKeyAlgorithm: publicKeyAlgorithmName(sig.PubKeyAlgo),
	}, nil
}
