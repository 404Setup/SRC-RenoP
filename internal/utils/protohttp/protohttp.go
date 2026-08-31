/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

// Package protohttp reads and writes bounded protobuf HTTP payloads.
package protohttp

import (
	"github.com/gofiber/fiber/v3"
	"google.golang.org/protobuf/proto"

	"renop/internal/utils"
)

// ContentType is the MIME type used for protobuf request/response bodies.
const ContentType = "application/x-protobuf"

// MaxRequestBodySize bounds control-plane protobuf requests without limiting
// streamed artifact uploads handled by the storage routes.
const MaxRequestBodySize = 1 << 20

// Write marshals m as protobuf and writes it with the protobuf content type.
func Write(c fiber.Ctx, m proto.Message) error {
	return WriteStatus(c, fiber.StatusOK, m)
}

// WriteStatus is Write with an explicit HTTP status code.
func WriteStatus(c fiber.Ctx, status int, m proto.Message) error {
	data, err := proto.Marshal(m)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("An unexpected error occurred while attempting to encode a Protobuf message")
	}
	c.Set(fiber.HeaderContentType, ContentType)
	return c.Status(status).Send(data)
}

// Read unmarshals a size-limited request body into m as protobuf.
func Read(c fiber.Ctx, m proto.Message) error {
	body, err := utils.ReadRequestBodyLimited(c, MaxRequestBodySize)
	if err != nil {
		return err
	}
	return proto.Unmarshal(body, m)
}
