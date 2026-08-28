/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

package npm

import "github.com/goccy/go-json"

type publishAttachment struct {
	ContentType string `json:"content_type"`
	Data        string `json:"data"`
	Length      int64  `json:"length"`
}

type publishDocument struct {
	ID          string                       `json:"_id"`
	Revision    string                       `json:"_rev"`
	Name        string                       `json:"name"`
	Description string                       `json:"description"`
	Access      string                       `json:"access"`
	DistTags    map[string]string            `json:"dist-tags"`
	Versions    map[string]json.RawMessage   `json:"versions"`
	Attachments map[string]publishAttachment `json:"_attachments"`
}

type registryError struct {
	Error  string `json:"error"`
	Reason string `json:"reason"`
}

type operationResponse struct {
	OK  bool   `json:"ok"`
	ID  string `json:"id,omitempty"`
	Rev string `json:"rev,omitempty"`
}
