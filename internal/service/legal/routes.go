/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

// Package legal serves configured policy documents and checks browser consent.
package legal

import (
	"github.com/gofiber/fiber/v3"

	"renop/internal/config"
	"renop/internal/core"
)

const (
	// ConsentCookie contains the public revision a browser explicitly accepted.
	ConsentCookie = "renop_legal_consent"
	// ConsentHeader allows non-browser clients to acknowledge the current policies.
	ConsentHeader = "X-Renop-Legal-Revision"
	// ConsentErrorCode is returned when the current privacy policy and terms were not accepted.
	ConsentErrorCode = "legal_consent_required"
)

// ErrConsentRequired rejects account entry until the current policies are accepted.
var ErrConsentRequired = fiber.NewError(fiber.StatusPreconditionRequired, "Accept the current privacy policy and terms of service")

// IsPublicPath identifies legal resources that remain readable with expired credentials.
func IsPublicPath(path string) bool {
	switch path {
	case "/privacy-policy", "/terms-of-service", "/legal-notice", "/api/legal", "/api/privacy-policy",
		"/api/legal/privacy-policy", "/api/legal/terms-of-service", "/api/legal/legal-notice":
		return true
	default:
		return false
	}
}

// RequireConsent prevents browser account actions without acceptance of the current policies.
func RequireConsent(c fiber.Ctx, state *core.AppState) error {
	if state == nil || state.Inner == nil || state.Inner.Config.Load() == nil {
		return fiber.ErrServiceUnavailable
	}
	revision := state.Inner.Config.Load().Legal.Revision()
	if c.Get(ConsentHeader, c.Cookies(ConsentCookie)) != revision {
		c.Set("X-Renop-Error-Code", ConsentErrorCode)
		return ErrConsentRequired
	}
	c.Locals("legal_revision", revision)
	return nil
}

// AcceptedRevision returns the policy revision checked for the current request.
func AcceptedRevision(c fiber.Ctx) string {
	value, _ := c.Locals("legal_revision").(string)
	return value
}

// SetupRoutes exposes policy metadata and bounded Markdown documents.
func SetupRoutes(router fiber.Router, state *core.AppState) {
	router.Get("/legal", func(c fiber.Ctx) error {
		c.Set(fiber.HeaderCacheControl, "no-store")
		value := state.Inner.Config.Load().Legal
		return c.JSON(fiber.Map{"revision": value.Revision(), "cookie_banner": value.CookieBanner})
	})
	router.Get("/legal/:document", func(c fiber.Ctx) error {
		return serveDocument(c, state.Inner.Config.Load().Legal, c.Params("document"))
	})
	router.Get("/privacy-policy", func(c fiber.Ctx) error {
		return serveDocument(c, state.Inner.Config.Load().Legal, "privacy-policy")
	})
}

func serveDocument(c fiber.Ctx, value config.LegalConfig, document string) error {
	var content string
	switch document {
	case "privacy-policy":
		content = value.PrivacyPolicy
	case "terms-of-service":
		content = value.TermsOfService
	case "legal-notice":
		content = value.LegalNotice
	default:
		return fiber.ErrNotFound
	}
	if content == "" || len(content) > config.MaxLegalDocumentBytes {
		return fiber.ErrServiceUnavailable
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	c.Set(fiber.HeaderContentType, "text/plain; charset=utf-8")
	c.Set(fiber.HeaderXContentTypeOptions, "nosniff")
	return c.SendString(content)
}
