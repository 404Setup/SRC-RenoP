/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package docker

const (
	DockerHeaderVersion = "Docker-Distribution-API-Version"
	DockerVersionValue  = "registry/2.0"
	DockerDigestHeader  = "Docker-Content-Digest"
	DockerUploadUUID    = "Docker-Upload-UUID"

	MediaTypeDockerManifest2      = "application/vnd.docker.distribution.manifest.v2+json"
	MediaTypeDockerManifestList   = "application/vnd.docker.distribution.manifest.list.v2+json"
	MediaTypeDockerConfig         = "application/vnd.docker.container.image.v1+json"
	MediaTypeDockerLayer          = "application/vnd.docker.image.rootfs.diff.tar.gzip"
	MediaTypeOCIManifest1         = "application/vnd.oci.image.manifest.v1+json"
	MediaTypeOCIIndex1            = "application/vnd.oci.image.index.v1+json"
	MediaTypeOCIConfig1           = "application/vnd.oci.image.config.v1+json"
	MediaTypeOCILayer1            = "application/vnd.oci.image.layer.v1.tar+gzip"
	MediaTypeOCILayerZstd         = "application/vnd.oci.image.layer.v1.tar+zstd"
	MediaTypeOCILayerUncompressed = "application/vnd.oci.image.layer.v1.tar"
	MediaTypeOctetStream          = "application/octet-stream"
)

// Descriptor describes a targeted content addressable blob in OCI/Docker specs.
type Descriptor struct {
	MediaType   string            `json:"mediaType"`
	Size        int64             `json:"size"`
	Digest      string            `json:"digest"`
	URLs        []string          `json:"urls,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	Platform    *Platform         `json:"platform,omitempty"`
}

// Platform describes the target architecture and operating system of an image.
type Platform struct {
	Architecture string   `json:"architecture"`
	OS           string   `json:"os"`
	OSVersion    string   `json:"os.version,omitempty"`
	OSFeatures   []string `json:"os.features,omitempty"`
	Variant      string   `json:"variant,omitempty"`
}

// ManifestSchema2 represents Docker Schema 2 Manifest or OCI Image Manifest.
type ManifestSchema2 struct {
	SchemaVersion int               `json:"schemaVersion"`
	MediaType     string            `json:"mediaType,omitempty"`
	Config        Descriptor        `json:"config"`
	Layers        []Descriptor      `json:"layers"`
	Annotations   map[string]string `json:"annotations,omitempty"`
}

// ManifestIndex represents Docker Manifest List or OCI Image Index.
type ManifestIndex struct {
	SchemaVersion int               `json:"schemaVersion"`
	MediaType     string            `json:"mediaType,omitempty"`
	Manifests     []Descriptor      `json:"manifests"`
	Annotations   map[string]string `json:"annotations,omitempty"`
}

// TagList is returned by the GET /v2/<name>/tags/list endpoint.
type TagList struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

// CatalogList is returned by the GET /v2/_catalog endpoint.
type CatalogList struct {
	Repositories []string `json:"repositories"`
}

// TokenResponse is returned by /v2/token or /v2/auth for Docker CLI authentication.
type TokenResponse struct {
	Token       string `json:"token"`
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	IssuedAt    string `json:"issued_at"`
}
