/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package cargodocs

import (
	"bytes"
	"errors"
	"fmt"
	"html"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v3"

	"renop/internal/core"
	"renop/internal/service/auth"
	"renop/internal/utils"
)

const cargodocTemplate = `<!DOCTYPE html>
<html lang="en">
    <head>
        <meta charset="UTF-8">
        <meta name="viewport" content="width=device-width, initial-scale=1.0">
        <title>{{CRATE_NAME}} {{CRATE_VERSION}} - RenoP Cargo Docs</title>
        <meta http-equiv="Content-Security-Policy" content="default-src 'self' 'unsafe-inline' 'unsafe-eval' data: blob:; frame-src 'self' data: blob:;">
        <style>
            :root {
                --nav-height: 48px;
                --bg-primary: #1f2937;
                --text-primary: #f9fafb;
                --accent: #e06c75;
                --border-color: #374151;
            }
            body {
                background-color: #111827;
                font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
                margin: 0;
            }
            .sticky-nav {
                position: fixed;
                display: flex;
                flex-direction: row;
                align-items: center;
                justify-content: space-between;
                top: 0;
                left: 0;
                width: 100vw;
                box-sizing: border-box;
                height: var(--nav-height);
                padding: 0 1.25rem;
                background-color: var(--bg-primary);
                color: var(--text-primary);
                border-bottom: 1px solid var(--border-color);
                z-index: 1000;
            }
            .doc {
                position: fixed;
                top: var(--nav-height);
                left: 0;
                width: 100%;
                height: calc(100vh - var(--nav-height));
                border: none;
            }
            .row {
                display: flex;
                align-items: center;
                gap: 1.25rem;
            }
            a {
                text-decoration: none;
                color: var(--text-primary);
                font-size: 0.95rem;
                transition: color 0.15s ease;
            }
            a:hover {
                color: var(--accent);
            }
            .title {
                display: flex;
                align-items: center;
                gap: 0.5rem;
                font-weight: 700;
                font-size: 1.1rem;
            }
            .crate-badge {
                display: inline-flex;
                align-items: center;
                gap: 0.35rem;
                padding: 0.2rem 0.6rem;
                border-radius: 9999px;
                background: rgba(224, 108, 117, 0.15);
                color: var(--accent);
                font-size: 0.85rem;
                font-weight: 600;
            }
            .version-tag {
                opacity: 0.85;
                font-size: 0.8rem;
            }
            .nav-actions {
                display: flex;
                align-items: center;
                gap: 1rem;
            }
            .btn-link {
                padding: 0.3rem 0.75rem;
                border-radius: 6px;
                border: 1px solid var(--border-color);
                background: rgba(255, 255, 255, 0.05);
                font-size: 0.85rem;
            }
            .btn-link:hover {
                background: rgba(255, 255, 255, 0.1);
            }
        </style>
    </head>
    <body>
        <nav class="sticky-nav">
            <div class="row">
                <a class="title" href="/">RenoP</a>
                <a href="{{PACKAGE_URL}}" class="crate-badge">
                    <span>{{CRATE_NAME}}</span>
                    <span class="version-tag">v{{CRATE_VERSION}}</span>
                </a>
            </div>
            <div class="nav-actions">
                <a href="{{PACKAGE_URL}}" class="btn-link">Back to Package</a>
                <a id="raw" class="btn-link">Raw docs</a>
            </div>
        </nav>
        <iframe id="cargodoc" class="doc" sandbox="allow-scripts allow-same-origin allow-popups allow-popups-to-escape-sandbox allow-top-navigation-by-user-activation"></iframe>
        <script>
            let base = window.location.pathname;
            if (!base.endsWith('/')) {
                base += '/';
            }
            let rawUrl = base + 'raw/index.html';
            let iframe = document.getElementById("cargodoc");
            iframe.src = rawUrl;
            document.getElementById('raw').href = rawUrl;

            function wireExternalLinks(doc) {
                if (!doc) return;
                doc.addEventListener('click', function(e) {
                    const link = e.target.closest('a');
                    if (!link) return;
                    const href = link.getAttribute('href');
                    if (!href) return;
                    if (/^(https?:|\/\/)/i.test(href)) {
                        link.target = '_blank';
                        link.rel = 'noopener noreferrer';
                    }
                }, true);
            }

            iframe.addEventListener('load', function() {
                try {
                    wireExternalLinks(iframe.contentDocument);
                } catch (_) {}
            });
        </script>
    </body>
</html>`

const rawCargodocCSP = "sandbox allow-scripts allow-same-origin allow-popups allow-popups-to-escape-sandbox allow-top-navigation-by-user-activation; " +
	"default-src 'self' data: blob:; " +
	"script-src 'self' 'unsafe-inline' 'unsafe-eval' data: blob:; " +
	"style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; " +
	"font-src 'self' data:; connect-src 'self' data: blob:; object-src 'none'; " +
	"base-uri 'none'; form-action 'none'; frame-ancestors 'self'"

func SetupCargodocRoutes(router fiber.Router, state *core.AppState) {
	router.Get("/cargodoc/:repo_name/:crate_name", func(c fiber.Ctx) error { return HandleCargodocPage(c, state) })
	router.Get("/cargodoc/:repo_name/:crate_name/*", func(c fiber.Ctx) error { return HandleCargodocPage(c, state) })
}

func HandleCargodocPage(c fiber.Ctx, state *core.AppState) error {
	repoName := c.Params("repo_name")
	crateName := c.Params("crate_name")
	restPath := c.Params("*")

	if !utils.IsValidRepositoryName(repoName) || crateName == "" {
		return c.Status(fiber.StatusBadRequest).SendString("Bad request")
	}

	sanitizedRest, ok := utils.SanitizePath(restPath)
	if !ok {
		return c.Status(fiber.StatusBadRequest).SendString("Bad request")
	}

	cfg := state.Inner.Config.Load()
	if !cfg.EnableCargodocPreview {
		return c.Status(fiber.StatusNotFound).SendString("Cargo docs preview is not enabled on this RenoP instance.")
	}

	repo, ok := cfg.Maven.Repositories[repoName]
	if !ok {
		return c.Status(fiber.StatusNotFound).SendString("Not found")
	}

	user := auth.GetUser(c)
	canRead := user.CheckReadPermission(repoName, sanitizedRest, repo.Visibility, true)
	if !canRead && !strings.EqualFold(repo.Visibility, "PUBLIC") {
		if strings.EqualFold(repo.Visibility, "HIDDEN") {
			canRead = true
		} else if db := state.GetDB(); db != nil && user.Username != "" && !strings.EqualFold(user.Username, "guest") {
			allowed, _ := db.HasCargoMembership(repoName, user.Username)
			canRead = allowed
		}
	}
	if !canRead {
		return c.Status(fiber.StatusNotFound).SendString("Not found")
	}

	parts := strings.Split(strings.Trim(sanitizedRest, "/"), "/")
	version := ""
	if len(parts) > 0 && parts[0] != "" && parts[0] != "raw" {
		version = parts[0]
	}

	db := state.GetDB()
	if version == "" {
		if db == nil {
			return c.Status(fiber.StatusNotFound).SendString("Database unavailable")
		}
		pkg, err := db.GetCargoPackage(repoName, strings.ReplaceAll(strings.ToLower(crateName), "_", "-"))
		if err != nil || pkg == nil {
			return c.Status(fiber.StatusNotFound).SendString("Cargo package not found")
		}
		targetVer := pkg.MaxVersion
		if targetVer == "" {
			details, err := db.GetCargoPackageDetails(repoName, pkg.NormalizedName, user.Username)
			if err == nil && details != nil && len(details.Versions) > 0 {
				targetVer = details.Versions[len(details.Versions)-1].Version
			}
		}
		if targetVer == "" {
			return c.Status(fiber.StatusNotFound).SendString("No published version for Cargo package")
		}
		return c.Redirect().To(fmt.Sprintf("/cargodoc/%s/%s/%s/", repoName, crateName, targetVer))
	}

	if before, after, ok0 := strings.Cut(sanitizedRest, "/raw/"); ok0 {
		subParts := strings.Split(strings.Trim(before, "/"), "/")
		ver := subParts[0]
		resource := after
		return ServeRawCargodoc(c, state, repoName, crateName, ver, resource)
	} else if strings.HasPrefix(sanitizedRest, "raw/") || sanitizedRest == "raw" {
		resource := strings.TrimPrefix(sanitizedRest, "raw")
		resource = strings.TrimPrefix(resource, "/")
		return ServeRawCargodoc(c, state, repoName, crateName, version, resource)
	}

	_, err := EnsureCargodocExtractedBlocking(state, repoName, crateName, version)
	if err != nil {
		if !errors.Is(err, fiber.ErrNotFound) && !errors.Is(err, os.ErrNotExist) {
			log.Printf("[Cargodoc] Documentation missing or extraction failed for repo=%s crate=%s version=%s: %v", repoName, crateName, version, err)
		}
		return c.Status(fiber.StatusNotFound).SendString("Documentation not found for package version")
	}

	packageURL := fmt.Sprintf("/%s/packages/%s", repoName, crateName)
	pageHTML := strings.NewReplacer(
		"{{CRATE_NAME}}", html.EscapeString(crateName),
		"{{CRATE_VERSION}}", html.EscapeString(version),
		"{{PACKAGE_URL}}", packageURL,
	).Replace(cargodocTemplate)

	c.Set(fiber.HeaderContentType, "text/html; charset=utf-8")
	return c.Status(fiber.StatusOK).SendString(pageHTML)
}

func ServeRawCargodoc(c fiber.Ctx, state *core.AppState, repoName, crateName, version, resource string) error {
	cfg := state.Inner.Config.Load()
	if !cfg.EnableCargodocPreview {
		return c.Status(fiber.StatusNotFound).SendString("Cargo docs preview is not enabled on this RenoP instance.")
	}

	sanitizedResource, ok := utils.SanitizePath(resource)
	if !ok {
		return c.Status(fiber.StatusBadRequest).SendString("Bad request")
	}

	cacheDir, err := EnsureCargodocExtractedBlocking(state, repoName, crateName, version)
	if err != nil {
		if errors.Is(err, fiber.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
			return c.Status(fiber.StatusNotFound).SendString("Not found")
		}
		log.Printf("[Cargodoc] Failed to extract raw cargodoc for repo=%s crate=%s version=%s resource=%s: %v", repoName, crateName, version, resource, err)
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	cacheDir = filepath.Clean(cacheDir)
	resourcePath := filepath.Clean(filepath.Join(cacheDir, sanitizedResource))
	if !utils.IsSubPath(cacheDir, resourcePath) {
		return c.Status(fiber.StatusForbidden).SendString("Forbidden")
	}

	info, err := os.Lstat(resourcePath)
	if err != nil {
		return c.Status(fiber.StatusNotFound).SendString("Not found")
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return c.Status(fiber.StatusForbidden).SendString("Forbidden")
	}
	if info.IsDir() {
		resourcePath = filepath.Join(resourcePath, "index.html")
		if !utils.IsSubPath(cacheDir, resourcePath) {
			return c.Status(fiber.StatusForbidden).SendString("Forbidden")
		}
		info, err = os.Lstat(resourcePath)
		if err != nil || info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return c.Status(fiber.StatusNotFound).SendString("Not found")
		}
	}

	c.Set(fiber.HeaderContentDisposition, "inline")
	c.Set(fiber.HeaderContentSecurityPolicy, rawCargodocCSP)
	c.Set(fiber.HeaderXContentTypeOptions, "nosniff")
	c.Set(fiber.HeaderReferrerPolicy, "no-referrer")

	lowerPath := strings.ToLower(resourcePath)
	if (strings.HasSuffix(lowerPath, ".html") || strings.HasSuffix(lowerPath, ".htm")) && info.Size() <= maxCargodocEntrySize {
		content, err := os.ReadFile(resourcePath)
		if err == nil {
			c.Set(fiber.HeaderContentType, "text/html; charset=utf-8")
			scriptTag := []byte(`<script>document.addEventListener('click',function(e){var a=e.target.closest('a');if(a&&/^(https?:|\/\/)/i.test(a.getAttribute('href')||'')){a.target='_blank';a.rel='noopener noreferrer';}},true);</script>`)
			if idx := bytes.LastIndex(content, []byte("</body>")); idx != -1 {
				modified := make([]byte, len(content)+len(scriptTag))
				copy(modified, content[:idx])
				copy(modified[idx:], scriptTag)
				copy(modified[idx+len(scriptTag):], content[idx:])
				return c.Status(fiber.StatusOK).Send(modified)
			} else if idx := bytes.LastIndex(content, []byte("</head>")); idx != -1 {
				modified := make([]byte, len(content)+len(scriptTag))
				copy(modified, content[:idx])
				copy(modified[idx:], scriptTag)
				copy(modified[idx+len(scriptTag):], content[idx:])
				return c.Status(fiber.StatusOK).Send(modified)
			}
			return c.Status(fiber.StatusOK).Send(content)
		}
	}

	return c.SendFile(resourcePath, fiber.SendFile{
		CacheDuration: -1,
		MaxAge:        3600,
	})
}
