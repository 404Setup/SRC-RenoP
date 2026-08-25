/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package frontend

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"go.yaml.in/yaml/v3"

	"renop/internal/config"
	"renop/internal/core"
)

func TestBundledAssetsEmbedded(t *testing.T) {
	files := []string{
		"index.html",
		"js/main.js",
		"css/style.css",
		"svg/logo.svg",
	}

	for _, file := range files {
		data, err := readAsset(file)
		if err != nil {
			t.Fatalf("expected %s to be embedded, got error: %v", file, err)
		}
		if len(data) == 0 {
			t.Fatalf("%s is empty", file)
		}
	}
}

func TestOpenAPIDocumentIncludesFrontendRoutes(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "web", "assets", "openapi.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Paths map[string]any `yaml:"paths"`
	}
	if err := yaml.Unmarshal(data, &document); err != nil {
		t.Fatalf("parse OpenAPI YAML: %v", err)
	}
	for _, path := range []string{
		"/api/users/{username}/profile", "/api/users/{username}/memberships",
		"/api/users/profiles", "/api/auth/profile",
		"/api/maven/repositories/{repo_name}/domains",
		"/api/maven/repositories/{repo_name}/packages",
		"/api/docker/repositories/{repo_name}/images",
	} {
		if _, exists := document.Paths[path]; !exists {
			t.Fatalf("OpenAPI document is missing %s", path)
		}
	}
}

func TestIndexHtmlUsesBundledAssets(t *testing.T) {
	data, err := readAsset("index.html")
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	html := string(data)
	for _, needle := range []string{
		`/css/style.css?v={{RENOP.HASH}}`,
		`/js/main.js?v={{RENOP.HASH}}`,
		`id="repository-search"`,
		`id="cargo-repository-view"`,
		`id="profile-trigger"`,
		`id="profile-menu"`,
		`data-i18n="nav.backHome"`,
		`data-profile-action="edit"`,
		`data-profile-tab="settings"`,
		`id="profile-public-view"`,
		`profile-settings-card`,
		`profile-collapsible-content`,
		`class="nav-title-logo"`,
	} {
		if !strings.Contains(html, needle) {
			t.Fatalf("index.html missing bundled asset reference %q", needle)
		}
	}
	if strings.Contains(html, "{{RENOP.CSS_LINKS}}") || strings.Contains(html, "{{RENOP.JS_LINKS}}") {
		t.Fatal("index.html still contains legacy CSS/JS link placeholders")
	}
	if strings.Contains(html, `id="cargo-packages-card"`) {
		t.Fatal("index.html still contains the obsolete Cargo package-management side card")
	}
	if strings.Contains(html, `id="tabs"`) || strings.Contains(html, `data-tab=`) {
		t.Fatal("index.html still exposes the removed main tab navigation")
	}
	for _, obsoleteHeader := range []string{`class="main-header"`, `class="header-text"`, `id="instance-url"`} {
		if strings.Contains(html, obsoleteHeader) {
			t.Fatalf("index.html still exposes obsolete repository header content %q", obsoleteHeader)
		}
	}
	if strings.Contains(html, `users.thTokenPrefix`) {
		t.Fatal("index.html still exposes access-token prefixes in the users table")
	}
}

func TestProfileUIKeepsStableIDsHiddenAndCentralizesHistoryRouting(t *testing.T) {
	profileSource, err := os.ReadFile(filepath.Join("renop-html", "js", "profile.js"))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"profile.user_id", "gpg_key_count", "t('profile.gpgKeys')"} {
		if strings.Contains(string(profileSource), forbidden) {
			t.Fatalf("profile UI still renders forbidden account metadata %q", forbidden)
		}
	}

	mainSource, err := os.ReadFile(filepath.Join("renop-html", "js", "main.js"))
	if err != nil {
		t.Fatal(err)
	}
	browserSource, err := os.ReadFile(filepath.Join("renop-html", "js", "browser.js"))
	if err != nil {
		t.Fatal(err)
	}
	const popstateRegistration = "addEventListener('popstate'"
	if count := strings.Count(string(mainSource), popstateRegistration); count != 1 {
		t.Fatalf("main.js popstate registration count = %d, want 1", count)
	}
	if strings.Contains(string(browserSource), popstateRegistration) {
		t.Fatal("browser.js still owns a competing popstate handler")
	}
}

func TestRegistryAsyncActionsRestoreInitiatingButtons(t *testing.T) {
	buttonSource, err := os.ReadFile(filepath.Join("renop-html", "js", "components", "button.js"))
	if err != nil {
		t.Fatal(err)
	}
	buttonText := string(buttonSource)
	for _, required := range []string{"export async function runButtonAction", "button.disabled = true", "button.disabled = false"} {
		if !strings.Contains(buttonText, required) {
			t.Fatalf("button action helper is missing %q", required)
		}
	}

	for _, sourcePath := range []string{
		filepath.Join("renop-html", "js", "browser", "docker.js"),
		filepath.Join("renop-html", "js", "browser", "maven.js"),
	} {
		source, readErr := os.ReadFile(sourcePath)
		if readErr != nil {
			t.Fatal(readErr)
		}
		text := string(source)
		if strings.Contains(text, "currentTarget.disabled") {
			t.Fatalf("%s accesses transient event.currentTarget after an asynchronous action", sourcePath)
		}
		if !strings.Contains(text, "runButtonAction") {
			t.Fatalf("%s does not use the shared asynchronous button action", sourcePath)
		}
	}

}

func TestRepositoryViewsUseSharedLifecycleAndCopyHelpers(t *testing.T) {
	helperSource, err := os.ReadFile(filepath.Join("renop-html", "js", "browser", "repository-view.js"))
	if err != nil {
		t.Fatal(err)
	}
	helperText := string(helperSource)
	for _, required := range []string{
		"export function ensureRepositoryView",
		"export function hideRepositoryView",
		"export function setRepositoryViewBusy",
		"export async function replaceRepositoryView",
		"export function createRepositoryBackButton",
		"export function formatRepositoryTimestamp",
	} {
		if !strings.Contains(helperText, required) {
			t.Fatalf("repository view helper is missing %q", required)
		}
	}

	for _, sourcePath := range []string{
		filepath.Join("renop-html", "js", "browser", "cargo.js"),
		filepath.Join("renop-html", "js", "browser", "docker.js"),
		filepath.Join("renop-html", "js", "browser", "maven.js"),
	} {
		source, readErr := os.ReadFile(sourcePath)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if !strings.Contains(string(source), "./repository-view.js") {
			t.Fatalf("%s does not use the shared repository view lifecycle", sourcePath)
		}
	}

	dockerSource, err := os.ReadFile(filepath.Join("renop-html", "js", "browser", "docker.js"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(dockerSource), "navigator.clipboard.writeText") {
		t.Fatal("Docker view still maintains a duplicate clipboard implementation")
	}
	copySource, err := os.ReadFile(filepath.Join("renop-html", "js", "browser", "copy-feedback.js"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(copySource), "originalChildren") {
		t.Fatal("shared copy feedback does not restore arbitrary button content")
	}
}

func TestDockerI18nCatalogsCoverUIAndAuditMessages(t *testing.T) {
	i18nRoot := filepath.Join("renop-html", "js", "i18n")
	keyPattern := regexp.MustCompile(`"(docker\.[^"]+|audit\.action\.DOCKER_[^"]+)"\s*:`)
	readKeys := func(path string) map[string]struct{} {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		keys := make(map[string]struct{})
		for _, match := range keyPattern.FindAllStringSubmatch(string(data), -1) {
			keys[match[1]] = struct{}{}
		}
		return keys
	}

	baseKeys := readKeys(filepath.Join(i18nRoot, "en-US", "docker.js"))
	locales, err := os.ReadDir(i18nRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, locale := range locales {
		if !locale.IsDir() {
			continue
		}
		localeKeys := readKeys(filepath.Join(i18nRoot, locale.Name(), "docker.js"))
		for key := range baseKeys {
			if _, exists := localeKeys[key]; !exists {
				t.Fatalf("Docker locale %s is missing %s", locale.Name(), key)
			}
		}
		for key := range localeKeys {
			if _, exists := baseKeys[key]; !exists {
				t.Fatalf("Docker locale %s has non-canonical key %s", locale.Name(), key)
			}
		}
	}

	usagePattern := regexp.MustCompile(`['"](docker\.[A-Za-z0-9_.]+)['"]`)
	for _, sourcePath := range []string{
		filepath.Join("renop-html", "js", "browser", "docker.js"),
		filepath.Join("renop-html", "js", "docker-errors.js"),
		filepath.Join("renop-html", "js", "docker-messages.js"),
	} {
		source, readErr := os.ReadFile(sourcePath)
		if readErr != nil {
			t.Fatal(readErr)
		}
		text := string(source)
		for _, match := range usagePattern.FindAllStringSubmatch(text, -1) {
			if match[1] == "docker.permissionL" {
				continue
			}
			if _, exists := baseKeys[match[1]]; !exists {
				t.Fatalf("%s uses missing Docker i18n key %s", sourcePath, match[1])
			}
		}
		for _, forbidden := range []string{"response.text()", "String(err.message", "String(error.message"} {
			if strings.Contains(text, forbidden) {
				t.Fatalf("%s exposes an unlocalized Docker response through %q", sourcePath, forbidden)
			}
		}
	}

	actionPattern := regexp.MustCompile(`"(DOCKER_[A-Z_]+)"`)
	usedActions := make(map[string]struct{})
	for _, sourcePath := range []string{
		filepath.Join("..", "..", "api", "docker.go"),
		filepath.Join("..", "docker", "handler.go"),
	} {
		source, readErr := os.ReadFile(sourcePath)
		if readErr != nil {
			t.Fatal(readErr)
		}
		for _, match := range actionPattern.FindAllStringSubmatch(string(source), -1) {
			if match[1] == "DOCKER_INVITE_" {
				usedActions["DOCKER_INVITE_ACCEPT"] = struct{}{}
				usedActions["DOCKER_INVITE_REJECT"] = struct{}{}
				continue
			}
			usedActions[match[1]] = struct{}{}
		}
	}
	for action := range usedActions {
		if _, exists := baseKeys["audit.action."+action]; !exists {
			t.Fatalf("Docker audit action %s has no i18n key", action)
		}
	}
	for key := range baseKeys {
		if !strings.HasPrefix(key, "audit.action.DOCKER_") {
			continue
		}
		action := strings.TrimPrefix(key, "audit.action.")
		if _, exists := usedActions[action]; !exists {
			t.Fatalf("Docker audit i18n key %s has no backend action", key)
		}
	}
}

func TestAssetsHashStableAndNonEmpty(t *testing.T) {
	h1 := GetAssetsHash()
	h2 := GetAssetsHash()
	if h1 == "" {
		t.Fatal("assets hash is empty")
	}
	if h1 != h2 {
		t.Fatalf("assets hash not stable: %q vs %q", h1, h2)
	}
}

func TestIndexAndConditionalAssetsRetainCacheSafetyHeaders(t *testing.T) {
	state := core.NewAppState()
	cfg := config.DefaultConfig()
	state.Inner.Config.Store(cfg)
	app := fiber.New()
	app.Get("/", func(c fiber.Ctx) error { return ServeIndex(c, state) })
	app.Get("/js/*", ServeJs)

	indexResponse, err := app.Test(httptest.NewRequest(http.MethodGet, "/", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer indexResponse.Body.Close()
	if cacheControl := indexResponse.Header.Get(fiber.HeaderCacheControl); cacheControl != frontendIndexCacheControl {
		t.Fatalf("index Cache-Control = %q, want %q", cacheControl, frontendIndexCacheControl)
	}

	assetResponse, err := app.Test(httptest.NewRequest(http.MethodGet, "/js/main.js", nil))
	if err != nil {
		t.Fatal(err)
	}
	etag := assetResponse.Header.Get(fiber.HeaderETag)
	_ = assetResponse.Body.Close()
	if etag == "" {
		t.Fatal("asset response is missing ETag")
	}

	conditionalRequest := httptest.NewRequest(http.MethodGet, "/js/main.js", nil)
	conditionalRequest.Header.Set(fiber.HeaderIfNoneMatch, etag)
	conditionalResponse, err := app.Test(conditionalRequest)
	if err != nil {
		t.Fatal(err)
	}
	defer conditionalResponse.Body.Close()
	if conditionalResponse.StatusCode != http.StatusNotModified {
		t.Fatalf("conditional asset status = %d, want %d", conditionalResponse.StatusCode, http.StatusNotModified)
	}
	if cacheControl := conditionalResponse.Header.Get(fiber.HeaderCacheControl); cacheControl != frontendAssetCacheControl {
		t.Fatalf("conditional asset Cache-Control = %q, want %q", cacheControl, frontendAssetCacheControl)
	}
	if pragma := conditionalResponse.Header.Get(fiber.HeaderPragma); pragma != "no-cache" {
		t.Fatalf("conditional asset Pragma = %q, want no-cache", pragma)
	}
}

func TestUserProfileRouteServesSPAIndex(t *testing.T) {
	state := core.NewAppState()
	state.Inner.Config.Store(config.DefaultConfig())
	app := fiber.New()
	SetupFrontendRoutes(app, state)
	for _, path := range []string{"/user/alice", "/user/alice/edit", "/user/alice/maven", "/user/alice/cargo", "/user/alice/docker"} {
		response, err := app.Test(httptest.NewRequest(http.MethodGet, path, nil))
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != http.StatusOK {
			_ = response.Body.Close()
			t.Fatalf("profile route %s status = %d, want 200", path, response.StatusCode)
		}
		if contentType := response.Header.Get(fiber.HeaderContentType); !strings.HasPrefix(contentType, "text/html") {
			_ = response.Body.Close()
			t.Fatalf("profile route %s Content-Type = %q, want text/html", path, contentType)
		}
		if err := response.Body.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestGenerateIndexHtmlIncludesEscapedLegalNoticeURL(t *testing.T) {
	cfg := config.DefaultFrontendConfig()
	cfg.LegalNoticeUrl = `https://example.com/legal?a=1&b="notice"`

	generated := string(GenerateIndexHtmlFromConfig(&cfg))
	if strings.Contains(generated, "{{RENOP.LEGAL_NOTICE_URL}}") {
		t.Fatal("generated HTML still contains the legal notice placeholder")
	}
	if !strings.Contains(generated, `data-url="https://example.com/legal?a=1&amp;b=&#34;notice&#34;"`) {
		t.Fatal("generated HTML does not contain the escaped legal notice URL")
	}
}

func TestGenerateIndexHtmlIncludesEscapedPublicSecurityFiling(t *testing.T) {
	cfg := config.DefaultFrontendConfig()
	cfg.PublicSecurityFiling = `京公网安备11000000000001号<script>`

	generated := string(GenerateIndexHtmlFromConfig(&cfg))
	if strings.Contains(generated, "{{RENOP.PUBLIC_SECURITY_FILING}}") {
		t.Fatal("generated HTML still contains the public security filing placeholder")
	}
	if !strings.Contains(generated, `京公网安备11000000000001号&lt;script&gt;`) {
		t.Fatal("generated HTML does not contain the escaped public security filing")
	}
}
