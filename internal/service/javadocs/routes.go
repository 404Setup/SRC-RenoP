/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package javadocs

import (
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v3"

	"renop/internal/core"
	"renop/internal/service/auth"
	"renop/internal/utils"
)

const javadocTemplate = `<!DOCTYPE html>
<html lang="en">
    <head>
        <meta charset="UTF-8">
        <meta name="viewport" content="width=device-width, initial-scale=1.0">
        <title>RenoP Javadocs</title>
        <meta http-equiv="Content-Security-Policy" content="default-src 'self' 'unsafe-inline' 'unsafe-eval' data: blob:; frame-src 'self';">
        <style>
            :root {
                --nav-height: 48px;
            }
            body {
                background-color: #f7f7f7;
                font-family: Arial, Helvetica, sans-serif;
                margin: 0;
            }
            .sticky-nav {
                position: fixed;
                display: flex;
                flex-direction: column;
                justify-content: center;
                top: 0;
                left: 0;
                width: calc(100vw - 2rem);
                height: var(--nav-height);
                padding-left: 1rem;
                padding-right: 1rem;
                background-color: #325064;
                color: #FFFFFF;
                z-index: 1000;
            }
            .doc {
                border-top: solid 3px #588DB0;
                position: fixed;
                top: var(--nav-height);
                left: 0;
                width: 100%;
                height: calc(100vh - var(--nav-height));
                border: none;
            }
            .row {
                display: flex;
                justify-content: flex-start;
                align-items: center;
            }
            a {
                text-decoration: none;
                color: white;
                width: auto;
                margin-right: 2rem;
            }
            .title {
                margin-right: 2rem;
            }
            a:hover {
                color: #e2dfdf;
            }
        </style>
    </head>
    <body>
        <div class="sticky-nav">
            <div class="row">
                <a class="title" href="/"><h3>RenoP</h3></a>
                <a id="raw"><h4>Raw docs</h4></a>
            </div>
        </div>
        <iframe id="javadoc" class="doc" sandbox="allow-scripts"></iframe>
        <script>
            let base = window.location.pathname;
            if (!base.endsWith('/')) {
                base += '/';
            }
            document.getElementById("javadoc").src = base + '{{UNPACKED_INDEX_PATH}}';
            document.getElementById('raw').href = base + '{{UNPACKED_INDEX_PATH}}';
        </script>
    </body>
</html>`

var javadocReplacer = strings.NewReplacer("{{UNPACKED_INDEX_PATH}}", "raw/index.html")

const rawJavadocCSP = "sandbox allow-scripts; default-src 'self' data: blob:; " +
	"script-src 'self' 'unsafe-inline' 'unsafe-eval' data: blob:; " +
	"style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; " +
	"font-src 'self' data:; connect-src 'none'; object-src 'none'; " +
	"base-uri 'none'; form-action 'none'; frame-ancestors 'self'"

func SetupJavadocRoutes(router fiber.Router, state *core.AppState) {
	router.Get("/javadoc/:repo_name/*", func(c fiber.Ctx) error { return HandleJavadocPage(c, state) })
}

func HandleJavadocPage(c fiber.Ctx, state *core.AppState) error {
	repoName := c.Params("repo_name")
	path1 := c.Params("*")
	sanitizedPath, ok := utils.SanitizePath(path1)
	if !ok {
		return c.Status(fiber.StatusBadRequest).SendString("Bad request")
	}

	user := auth.GetUser(c)

	cfg := state.Inner.Config.Load()
	if !cfg.EnableJavadocPreview {
		return c.Status(fiber.StatusNotFound).SendString("Not found")
	}
	repo, ok := cfg.Maven.Repositories[repoName]
	if !ok {
		return c.Status(fiber.StatusNotFound).SendString("Not found")
	}

	isRoot := strings.HasSuffix(sanitizedPath, "/") || sanitizedPath == ""
	isVisible := user.CheckReadPermission(repoName, sanitizedPath, repo.Visibility, isRoot)
	if !isVisible {
		return c.Status(fiber.StatusNotFound).SendString("Not found")
	}

	if before, after, ok0 := strings.Cut(sanitizedPath, "/raw/"); ok0 {
		gav := before
		resource := after
		return ServeRawJavadoc(c, state, repoName, gav, resource)
	}

	_, err := ensureJavadocExtracted(state, repoName, sanitizedPath, true)
	if err != nil {
		if errors.Is(err, fiber.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
			return c.Status(fiber.StatusNotFound).SendString("Not found")
		}
		log.Printf("[Javadoc] Failed to extract javadoc for repo=%s path=%s: %v", repoName, sanitizedPath, err)
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	htmlReplaced := javadocReplacer.Replace(javadocTemplate)
	c.Set(fiber.HeaderContentType, "text/html; charset=utf-8")
	return c.Status(fiber.StatusOK).SendString(htmlReplaced)
}

func ServeRawJavadoc(c fiber.Ctx, state *core.AppState, repoName string, gav string, resource string) error {
	cfg := state.Inner.Config.Load()
	if !cfg.EnableJavadocPreview {
		return c.Status(fiber.StatusNotFound).SendString("Javadocs preview is not enabled on this RenoP instance.")
	}

	sanitizedResource, ok := utils.SanitizePath(resource)
	if !ok {
		return c.Status(fiber.StatusBadRequest).SendString("Bad request")
	}

	cacheDir, err := ensureJavadocExtracted(state, repoName, gav, false)
	if err != nil {
		if errors.Is(err, fiber.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
			return c.Status(fiber.StatusNotFound).SendString("Not found")
		}
		log.Printf("[Javadoc] Failed to extract raw javadoc for repo=%s gav=%s resource=%s: %v", repoName, gav, resource, err)
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
	c.Set(fiber.HeaderContentSecurityPolicy, rawJavadocCSP)
	c.Set(fiber.HeaderXContentTypeOptions, "nosniff")
	c.Set(fiber.HeaderReferrerPolicy, "no-referrer")
	return c.SendFile(resourcePath, fiber.SendFile{
		CacheDuration: -1,
		MaxAge:        3600,
	})
}
