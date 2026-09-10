/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

// Package protohttp reads and writes bounded protobuf and ProtoJSON HTTP payloads.
package protohttp

import (
	"mime"

	"github.com/gofiber/fiber/v3"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"renop/internal/utils"
)

// ContentType is the MIME type used for protobuf request/response bodies.
const ContentType = "application/x-protobuf"

// MaxRequestBodySize bounds control-plane requests without limiting
// streamed artifact uploads handled by the storage routes.
const MaxRequestBodySize = 1 << 20

// Write encodes m using the requested response representation.
func Write(c fiber.Ctx, m proto.Message) error {
	return WriteStatus(c, fiber.StatusOK, m)
}

// WriteStatus is Write with an explicit HTTP status code.
func WriteStatus(c fiber.Ctx, status int, m proto.Message) error {
	contentType := c.Accepts(ContentType, fiber.MIMEApplicationJSON, "application/protobuf")
	if contentType == "" {
		contentType = ContentType
	}
	c.Vary(fiber.HeaderAccept)
	var data []byte
	var err error
	if contentType == fiber.MIMEApplicationJSON {
		data, err = (protojson.MarshalOptions{UseProtoNames: true, EmitDefaultValues: true}).Marshal(m)
	} else {
		data, err = proto.Marshal(m)
	}
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to encode response")
	}
	c.Set(fiber.HeaderContentType, contentType)
	return c.Status(status).Send(data)
}

// Read decodes a size-limited request according to Content-Type; legacy untyped bodies use protobuf.
func Read(c fiber.Ctx, m proto.Message) error {
	body, err := utils.ReadRequestBodyLimited(c, MaxRequestBodySize)
	if err != nil {
		return err
	}
	contentType, _, err := mime.ParseMediaType(c.Get(fiber.HeaderContentType, ContentType))
	if err != nil {
		return fiber.ErrBadRequest
	}
	switch contentType {
	case fiber.MIMEApplicationJSON:
		return (protojson.UnmarshalOptions{RecursionLimit: 64}).Unmarshal(body, m)
	case ContentType, "application/protobuf", fiber.MIMEOctetStream:
		return proto.Unmarshal(body, m)
	default:
		return fiber.ErrUnsupportedMediaType
	}
}
