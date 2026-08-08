/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package api

import (
	"errors"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v3"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/auth"
	"renop/internal/utils"
)

func countShortCharacters(s string) int {
	count := 0
	for _, c := range s {
		switch c {
		case 'i', 'j', 'l', 'f', 't', 'I', '1', '.', ',', ':', ';':
			count++
		}
	}
	return count
}

func LatestBadge(c fiber.Ctx, state *core.AppState) error {
	repoName := c.Params("repo_name")
	gav := c.Params("*")

	sanitizedGav, ok := utils.SanitizePath(gav)
	if !ok || sanitizedGav == "" {
		return c.Status(fiber.StatusBadRequest).SendString("Bad path")
	}

	user := auth.GetUser(c)
	cfg := state.Inner.Config.Load().(*config.Config)
	repo, exists := cfg.Maven.Repositories[repoName]
	if !exists {
		return c.Status(fiber.StatusNotFound).SendString("Not found")
	}

	if !user.CheckReadPermission(repoName, sanitizedGav, repo.Visibility, false) {
		return c.Status(fiber.StatusNotFound).SendString("Not found")
	}

	metadata, err := FindMetadata(state, repoName, sanitizedGav)
	if err != nil {
		if errors.Is(err, fiber.ErrNotFound) {
			return c.Status(fiber.StatusNotFound).SendString("Not found")
		}
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	query := new(BadgeQuery)
	if err := c.Bind().Query(query); err != nil {
		return c.Status(fiber.StatusBadRequest).SendString("Bad request")
	}

	_, versions := FindVersionsInternal(metadata, query.Filter, true)
	if len(versions) == 0 {
		return c.Status(fiber.StatusNotFound).SendString("Not found")
	}

	version := versions[len(versions)-1]

	name := repoName
	if query.Name != nil {
		name = *query.Name
	}

	prefix := ""
	if query.Prefix != nil {
		prefix = *query.Prefix
	}

	value := prefix + version

	color := "4c1"
	if query.Color != nil && *query.Color != "" {
		cVal := *query.Color
		safe := true
		for _, r := range cVal {
			if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '#') {
				safe = false
				break
			}
		}
		if safe {
			color = strings.TrimPrefix(cVal, "#")
		}
	}
	escapedColor := utils.EscapeXML(color)

	padding := 11
	textPadding := 110

	nameShortCharacters := countShortCharacters(name)
	nameWidth := len(name)*6 - nameShortCharacters + 2*padding
	nameTextLength := len(name)*60 - nameShortCharacters*10

	valueShortCharacters := countShortCharacters(value)
	valueWidth := len(value)*6 - valueShortCharacters + 2*padding
	valueTextLength := len(value)*60 - valueShortCharacters*10

	fullWidth := valueWidth + nameWidth

	nx := textPadding + nameTextLength/2
	vx := nameTextLength + valueTextLength/2 + 3*textPadding

	escapedName := utils.EscapeXML(name)
	escapedValue := utils.EscapeXML(value)

	fullWidthStr := strconv.Itoa(fullWidth)
	nameWidthStr := strconv.Itoa(nameWidth)
	valueWidthStr := strconv.Itoa(valueWidth)
	nxStr := strconv.Itoa(nx)
	nameTextLengthStr := strconv.Itoa(nameTextLength)
	vxStr := strconv.Itoa(vx)
	valueTextLengthStr := strconv.Itoa(valueTextLength)

	badge := "\n" +
		`<svg xmlns="http://www.w3.org/2000/svg" width="` + fullWidthStr + `" height="20" role="img" aria-label="` + escapedName + `: ` + escapedValue + `">
    <title>` + escapedName + `: ` + escapedValue + `</title>
    <linearGradient id="s" x2="0" y2="100%">
        <stop offset="0" stop-color="#bbb" stop-opacity=".1"/><stop offset="1" stop-opacity=".1"/>
    </linearGradient>
    <clipPath id="r">
        <rect width="` + fullWidthStr + `" height="20" rx="3" fill="#fff"/>
    </clipPath>
    <g clip-path="url(#r)">
        <rect width="` + nameWidthStr + `" height="20" fill="#555"/>
        <rect x="` + nameWidthStr + `" width="` + valueWidthStr + `" height="20" fill="#` + escapedColor + `"/>
        <rect width="` + fullWidthStr + `" height="20" fill="url(#s)"/>
    </g>
    <g fill="#fff" text-anchor="middle" font-family="Verdana,Geneva,DejaVu Sans,sans-serif" text-rendering="geometricPrecision" font-size="110">
        <text x="` + nxStr + `" y="150" fill="#010101" fill-opacity=".3" transform="scale(.1)" textLength="` + nameTextLengthStr + `">
            ` + escapedName + `
        </text>
        <text x="` + nxStr + `" y="140" transform="scale(.1)" fill="#fff" textLength="` + nameTextLengthStr + `">
            ` + escapedName + `
        </text>
        <text x="` + vxStr + `" y="150" fill="#010101" fill-opacity=".3" transform="scale(.1)" textLength="` + valueTextLengthStr + `">
            ` + escapedValue + `
        </text>
        <text x="` + vxStr + `" y="140" transform="scale(.1)" fill="#fff" textLength="` + valueTextLengthStr + `">
            ` + escapedValue + `
        </text>
    </g>
</svg>`

	c.Set(fiber.HeaderContentType, "image/svg+xml")
	c.Set(fiber.HeaderContentDisposition, `inline; filename="latest-badge.svg"`)

	return c.SendString(badge)
}
