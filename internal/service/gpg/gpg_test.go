/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package gpg

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rsa"
	"errors"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/packet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
	"renop/internal/testutil"
	"renop/internal/utils"
)

func testGPGState(t *testing.T) (*core.AppState, *database.DB) {
	t.Helper()
	cfg := config.DefaultConfig()
	cfg.Database = config.DatabaseConfig{
		Driver:       "sqlite",
		Dsn:          filepath.Join(testutil.TempDir(t), "gpg.db"),
		MaxOpenConns: 1,
		MaxIdleConns: 1,
	}
	db, err := database.InitDB(cfg.Database)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	state := core.NewAppState()
	state.Inner.Config.Store(cfg)
	state.Inner.DB = db
	return state, db
}

func testSigningEntity(t *testing.T) (*openpgp.Entity, *core.GPGPublicKey, []string) {
	t.Helper()
	entity, err := openpgp.NewEntity("Test Signer", "", "signer@example.test", &packet.Config{
		Algorithm:   packet.PubKeyAlgoEdDSA,
		DefaultHash: crypto.SHA256,
	})
	require.NoError(t, err)

	var serialized bytes.Buffer
	require.NoError(t, entity.Serialize(&serialized))
	publicEntities, err := openpgp.ReadKeyRing(bytes.NewReader(serialized.Bytes()))
	require.NoError(t, err)
	require.Len(t, publicEntities, 1)
	key, aliases, err := entityToPublicKey(publicEntities[0], time.Now())
	require.NoError(t, err)
	return entity, key, aliases
}

func TestGPGArtifactPolicy(t *testing.T) {
	for _, artifact := range []string{
		"demo.jar",
		"demo.pom",
		"demo.module",
		"org/example/DEMO.JAR",
	} {
		assert.True(t, IsProtectedArtifact(artifact), artifact)
	}
	for _, artifact := range []string{
		"demo.jar.asc",
		"demo.jar.sha256",
		"demo.zip",
		"maven-metadata.xml",
	} {
		assert.False(t, IsProtectedArtifact(artifact), artifact)
	}

	artifact, ok := ArtifactForDetachedSignature(`org\example\demo.jar.asc`)
	assert.True(t, ok)
	assert.Equal(t, "org/example/demo.jar", artifact)
	_, ok = ArtifactForDetachedSignature("demo.zip.asc")
	assert.False(t, ok)
	assert.Equal(t, ArtifactKey("releases", "org/example/demo.jar"), ArtifactKey("releases", `org\example\demo.jar`))
}

func TestNormalizeKeyReference(t *testing.T) {
	value, err := NormalizeKeyReference(" 0x0123 4567 89ab cdef ")
	require.NoError(t, err)
	assert.Equal(t, "0123456789ABCDEF", value)

	for _, invalid := range []string{"", "12345678", "not-a-key-reference", "0123456789ABCDEG"} {
		_, err := NormalizeKeyReference(invalid)
		assert.ErrorIs(t, err, errInvalidKeyReference, invalid)
	}
}

func TestValidateKeyServers(t *testing.T) {
	servers, err := ValidateKeyServers([]string{
		"https://keyserver.ubuntu.com/",
		"https://KEYSERVER.UBUNTU.COM",
		"https://keys.openpgp.org",
	})
	require.NoError(t, err)
	assert.Equal(t, []string{
		"https://keyserver.ubuntu.com",
		"https://keys.openpgp.org",
	}, servers)

	for _, invalid := range [][]string{
		nil,
		{"http://keyserver.example"},
		{"https://user:key@keyserver.example"},
		{"https://keyserver.example/path"},
		{"https://keyserver.example?query=value"},
	} {
		_, err := ValidateKeyServers(invalid)
		assert.Error(t, err)
	}
}

func TestKeyServerAddressMustBePublic(t *testing.T) {
	for _, value := range []string{
		"127.0.0.1",
		"10.0.0.1",
		"100.64.0.1",
		"169.254.169.254",
		"192.0.2.1",
		"::1",
		"fc00::1",
		"fe80::1",
		"64:ff9b::7f00:1",
		"2002:7f00:1::",
	} {
		assert.False(t, utils.IsPublicIP(net.ParseIP(value)), value)
	}
	for _, value := range []string{"1.1.1.1", "2606:4700:4700::1111"} {
		assert.True(t, utils.IsPublicIP(net.ParseIP(value)), value)
	}
}

func TestValidatedKeyServerAddressesPreferIPv4AndRejectMixedPrivateDNS(t *testing.T) {
	addresses, err := validatedKeyServerAddresses([]net.IP{
		net.ParseIP("2606:4700:4700::1111"),
		net.ParseIP("1.1.1.1"),
		net.ParseIP("8.8.8.8"),
	}, "443")
	require.NoError(t, err)
	assert.Equal(t, []string{
		"1.1.1.1:443",
		"8.8.8.8:443",
		"[2606:4700:4700::1111]:443",
	}, addresses)

	_, err = validatedKeyServerAddresses([]net.IP{
		net.ParseIP("1.1.1.1"),
		net.ParseIP("127.0.0.1"),
	}, "443")
	assert.Error(t, err)
}

func TestAcceptedSigningKeyRejectsWeakRSAAndLegacyAlgorithms(t *testing.T) {
	rsaPacket := func(bits int) *packet.PublicKey {
		modulus := new(big.Int).SetBit(new(big.Int), bits-1, 1)
		return &packet.PublicKey{
			PubKeyAlgo: packet.PubKeyAlgoRSA,
			PublicKey:  &rsa.PublicKey{N: modulus, E: 65537},
		}
	}
	assert.False(t, isAcceptedSigningKey(rsaPacket(1024)))
	assert.True(t, isAcceptedSigningKey(rsaPacket(2048)))
	assert.False(t, isAcceptedSigningKey(&packet.PublicKey{PubKeyAlgo: packet.PubKeyAlgoDSA}))
	assert.True(t, isAcceptedSigningKey(&packet.PublicKey{PubKeyAlgo: packet.PubKeyAlgoEdDSA}))
}

func TestVerifyDetachedUsesUploadersRegisteredKey(t *testing.T) {
	state, db := testGPGState(t)
	signer, key, aliases := testSigningEntity(t)
	require.NoError(t, db.RegisterUserGPGKey("alice", key.KeyID, key, aliases))

	artifact := []byte("signed Maven artifact")
	var armoredSignature bytes.Buffer
	require.NoError(t, openpgp.ArmoredDetachSign(
		&armoredSignature,
		signer,
		bytes.NewReader(artifact),
		&packet.Config{DefaultHash: crypto.SHA256},
	))

	record, err := VerifyDetached(
		context.Background(),
		state,
		"Alice",
		bytes.NewReader(artifact),
		armoredSignature.Bytes(),
		"releases",
		"org/example/demo/1.0/demo-1.0.jar",
	)
	require.NoError(t, err)
	assert.Equal(t, key.Fingerprint, record.Fingerprint)
	assert.Equal(t, "alice", record.Uploader)
	assert.Equal(t, "SHA-256", record.HashAlgorithm)
	assert.NotEmpty(t, record.PublicKeyAlgorithm)

	_, err = VerifyDetached(
		context.Background(), state, "alice", bytes.NewReader([]byte("tampered")),
		armoredSignature.Bytes(), "releases", "org/example/demo/1.0/demo-1.0.jar",
	)
	assert.ErrorIs(t, err, ErrSignatureInvalid)

	_, err = VerifyDetached(
		context.Background(), state, "bob", bytes.NewReader(artifact),
		armoredSignature.Bytes(), "releases", "org/example/demo/1.0/demo-1.0.jar",
	)
	assert.True(t, errors.Is(err, ErrSigningKeyUnregistered))

	appendedSignature := append(append([]byte(nil), armoredSignature.Bytes()...), armoredSignature.Bytes()...)
	_, err = VerifyDetached(
		context.Background(), state, "alice", bytes.NewReader(artifact),
		appendedSignature, "releases", "org/example/demo/1.0/demo-1.0.jar",
	)
	assert.ErrorIs(t, err, ErrSignatureInvalid)
}

func TestVerifyDetachedAcceptsExistingEd25519MavenSignature(t *testing.T) {
	const fingerprint = "1462C0512352DEC38A39D0793586B4EB0FDA2EA9"
	const publicKey = `-----BEGIN PGP PUBLIC KEY BLOCK-----
Comment: Hostname:
Version: Hockeypuck 2.2

xjMEZ1nCtxYJKwYBBAHaRw8BAQdAKgNA7iyiQcEm2hPXDS15pQNyMldm44bd62qw
LOswryfNNjQwNFNldHVwIDwxNTMzNjY2NTErNDA0U2V0dXBAdXNlcnMubm9yZXBs
eS5naXRodWIuY29tPsKZBBMWCgBBFiEEFGLAUSNS3sOKOdB5NYa06w/aLqkFAmdZ
wrcCGwMFCQWkN4kFCwkIBwICIgIGFQoJCAsCBBYCAwECHgcCF4AACgkQNYa06w/a
LqmmdgD+M2r/7TvwGhgvPwwyAQt7GRPdunNF7ulcMtlHHwWTfH4A/juEH/9kNf3c
8BuNcPCI7VktNKEr2PbawIOdd/40yhEKzjgEZ1nCtxIKKwYBBAGXVQEFAQEHQHzo
lsbo+YGGiVu+QASLsDyatsRqC+ZCbQ/uvxb5lXtsAwEIB8J+BBgWCgAmFiEEFGLA
USNS3sOKOdB5NYa06w/aLqkFAmdZwrcCGwwFCQWkN4kACgkQNYa06w/aLqkXVAD+
ODA14KzDrVGKylXprhRwbnpRDSUSEZBowoGbiiEnYmkA/1+KWIDrEZayvd7whEnI
1EecIW3G+3rNWjZRk7+G3zMK
=5JtL
-----END PGP PUBLIC KEY BLOCK-----`
	const detachedSignature = `-----BEGIN PGP SIGNATURE-----
Version: BCPG v@RELEASE_NAME@

iF4EABYKAAYFAmftMrEACgkQNYa06w/aLqmhtAD+KgahJ5lT/GBkFXGi3hVCAoim
NzImNwPj1DqY6jUso2QA+wXia/9TgEX7cim0GX8uj3tt16G9CvqvKt6FU0MjKM4H
=CLUz
-----END PGP SIGNATURE-----`
	const pom = `<?xml version="1.0" encoding="UTF-8"?>
<project xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 https://maven.apache.org/xsd/maven-4.0.0.xsd" xmlns="http://maven.apache.org/POM/4.0.0"
    xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <!-- This module was also published with a richer model, Gradle metadata,  -->
  <!-- which should be used instead. Do not delete the following line which  -->
  <!-- is to indicate to Gradle or any Gradle module metadata file consumer  -->
  <!-- that they should prefer consuming it instead. -->
  <!-- do_not_remove: published-with-gradle-metadata -->
  <modelVersion>4.0.0</modelVersion>
  <groupId>one.tranic</groupId>
  <artifactId>t-proxy</artifactId>
  <version>1.0.1</version>
  <name>TProxy</name>
  <description>Basic Development Library</description>
  <url>https://github.com/404Setup/t-proxy</url>
  <inceptionYear>2025</inceptionYear>
  <licenses>
    <license>
      <name>The Apache License, Version 2.0</name>
      <url>https://www.apache.org/licenses/LICENSE-2.0.txt</url>
      <distribution>https://www.apache.org/licenses/LICENSE-2.0.txt</distribution>
    </license>
  </licenses>
  <developers>
    <developer>
      <id>404</id>
      <name>404Setup</name>
      <url>https://github.com/404Setup</url>
    </developer>
  </developers>
  <scm>
    <connection>scm:git:git://github.com/404Setup/t-proxy.git</connection>
    <developerConnection>scm:git:ssh://git@github.com/404Setup/t-proxy.git</developerConnection>
    <url>https://github.com/404Setup/t-proxy</url>
  </scm>
</project>
`

	key, aliases, err := parsePublicKey([]byte(publicKey), fingerprint, time.Now())
	require.NoError(t, err)
	assert.Equal(t, fingerprint, key.Fingerprint)

	state, db := testGPGState(t)
	require.NoError(t, db.RegisterUserGPGKey("alice", fingerprint, key, aliases))
	record, err := VerifyDetached(
		context.Background(),
		state,
		"alice",
		bytes.NewReader([]byte(strings.ReplaceAll(pom, "\n", "\r\n"))),
		[]byte(detachedSignature),
		"releases",
		"one/tranic/t-proxy/1.0.1/t-proxy-1.0.1.pom",
	)
	require.NoError(t, err)
	assert.Equal(t, fingerprint, record.Fingerprint)
	assert.Equal(t, "3586B4EB0FDA2EA9", record.KeyID)
	assert.Equal(t, "EdDSA", record.PublicKeyAlgorithm)
	assert.Equal(t, "SHA-512", record.HashAlgorithm)
}

func TestFetchExistingEd25519PublicKeyIntegration(t *testing.T) {
	if os.Getenv("RENOP_GPG_LIVE_TEST") != "1" {
		t.Skip("set RENOP_GPG_LIVE_TEST=1 to query the configured public key servers")
	}
	const fingerprint = "1462C0512352DEC38A39D0793586B4EB0FDA2EA9"
	cfg := config.DefaultConfig()
	if proxyURL := os.Getenv("RENOP_GPG_LIVE_PROXY"); proxyURL != "" {
		cfg.Proxy = config.ProxyConfig{
			Selected: "integration",
			Proxies: []config.OutboundProxy{{
				Name: "integration",
				URL:  proxyURL,
			}},
		}
	}
	key, aliases, err := fetchPublicKey(context.Background(), cfg, fingerprint)
	require.NoError(t, err)
	assert.Equal(t, fingerprint, key.Fingerprint)
	assert.Contains(t, aliases, fingerprint)
}
