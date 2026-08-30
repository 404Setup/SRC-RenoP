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
	"bytes"
	"errors"
	"io"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"
)

const maxCargoAPIRequestSize = 16 << 10

func decodeJSON(c fiber.Ctx, destination any) error {
	var reader io.Reader
	if stream := c.Request().BodyStream(); stream != nil {
		reader = stream
	} else {
		reader = bytes.NewReader(c.Request().Body())
	}
	limited := &io.LimitedReader{R: reader, N: maxCargoAPIRequestSize + 1}
	decoder := json.NewDecoder(limited)
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("request must contain one JSON value")
	}
	if limited.N <= 0 {
		return errors.New("request is too large")
	}
	return nil
}
