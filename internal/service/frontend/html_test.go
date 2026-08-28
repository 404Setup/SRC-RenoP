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
	"bytes"
	"io/fs"
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
	"renop/internal/service/audit"
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
		"/api/maven/domains", "/api/maven/repositories/{repo_name}/domains",
		"/api/maven/repositories/{repo_name}/packages",
		"/api/docker/repositories/{repo_name}/images",
		"/api/npm/repositories/{repo_name}/packages",
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
		`id="npm-repository-view"`,
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

func TestRepositoryTeamsShareUserSuggestionController(t *testing.T) {
	controllerSource, err := os.ReadFile(filepath.Join("renop-html", "js", "browser", "user-suggestions.js"))
	if err != nil {
		t.Fatal(err)
	}
	controllerText := string(controllerSource)
	for _, required := range []string{
		"export class RepositoryUserSuggestions",
		"aria-activedescendant",
		"opens-upward",
		"handleDocumentClick",
		"handleViewportChange",
	} {
		if !strings.Contains(controllerText, required) {
			t.Fatalf("repository user suggestion controller is missing %q", required)
		}
	}

	for _, sourcePath := range []string{
		filepath.Join("renop-html", "js", "browser", "cargo.js"),
		filepath.Join("renop-html", "js", "browser", "docker.js"),
	} {
		source, readErr := os.ReadFile(sourcePath)
		if readErr != nil {
			t.Fatal(readErr)
		}
		text := string(source)
		if !strings.Contains(text, "RepositoryUserSuggestions") {
			t.Fatalf("%s does not use the shared user suggestion controller", sourcePath)
		}
		for _, obsolete := range []string{"ensureInviteSuggestionPanel", "positionInviteSuggestions", "inviteSuggestionTimer"} {
			if strings.Contains(text, obsolete) {
				t.Fatalf("%s retains duplicate autocomplete state %q", sourcePath, obsolete)
			}
		}
	}

	componentsCSS, err := os.ReadFile(filepath.Join("renop-html", "css", "components.css"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(componentsCSS), "components/user-suggestions.css") {
		t.Fatal("shared user suggestion styles are not part of the component bundle")
	}
	for _, sourcePath := range []string{
		filepath.Join("renop-html", "css", "browser", "cargo.css"),
		filepath.Join("renop-html", "css", "browser", "docker.css"),
	} {
		source, readErr := os.ReadFile(sourcePath)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if strings.Contains(string(source), "-user-suggestion") {
			t.Fatalf("%s retains format-specific autocomplete CSS", sourcePath)
		}
	}
}

func TestMavenDomainsUseGlobalAccountCenter(t *testing.T) {
	indexSource, err := os.ReadFile(filepath.Join("renop-html", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(indexSource), `data-account-action="maven-domains"`) {
		t.Fatal("account menu is missing global Maven domain settings")
	}
	mavenSource, err := os.ReadFile(filepath.Join("renop-html", "js", "browser", "maven.js"))
	if err != nil {
		t.Fatal(err)
	}
	mavenText := string(mavenSource)
	for _, required := range []string{
		"export function openMavenDomainCenter", "export async function loadMavenDomainCenterPage",
		"export function mavenDomainRouteFromPath", "view: 'managed'", "domainCenterPagination",
		"/api/maven/repositories/${encodeURIComponent(repository)}/domains", "maven.inviteRequired",
	} {
		if !strings.Contains(mavenText, required) {
			t.Fatalf("global Maven domain UI is missing %q", required)
		}
	}
	if !strings.Contains(string(indexSource), `id="tab-content-maven-domains"`) ||
		!strings.Contains(string(indexSource), `id="maven-domain-home"`) {
		t.Fatal("Maven domain settings are missing the routed page or home navigation")
	}
	if strings.Contains(mavenText, "maven-domain-center-dialog") {
		t.Fatal("Maven domain settings still open in a dialog")
	}
	if strings.Contains(mavenText, "/api/maven/repositories/${encodeURIComponent(repository)}/domains`, {") {
		t.Fatal("Maven domain UI still mutates repository-scoped domains")
	}
	if strings.Contains(mavenText, ": {artifacts: []}") || !strings.Contains(mavenText, "await readArtifactPage(artifactsResponse)") {
		t.Fatal("Maven domain UI can still report a failed artifact request as an empty catalog")
	}
	if !strings.Contains(mavenText, "members.length === 0 && !administrator") {
		t.Fatal("administrators cannot assign the first member of an imported Maven domain")
	}
	for _, detailSection := range []string{"domainInformationSection(details", "artifactInformationSection(details"} {
		if !strings.Contains(mavenText, detailSection) {
			t.Fatalf("Maven detail pages are missing %q", detailSection)
		}
	}
	repositoryViewSource, err := os.ReadFile(filepath.Join("renop-html", "js", "browser", "repository-view.js"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(repositoryViewSource), "export function createRepositoryFactsSection") {
		t.Fatal("repository detail pages are missing the shared metadata facts component")
	}
	messageSource, err := os.ReadFile(filepath.Join("renop-html", "js", "maven-messages.js"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(messageSource), "/api/maven/invitations/") {
		t.Fatal("Maven invitation actions still depend on a repository")
	}
}

func TestBackendOfflineDetectionRequiresForegroundConfirmation(t *testing.T) {
	mainSource, err := os.ReadFile(filepath.Join("renop-html", "js", "main.js"))
	if err != nil {
		t.Fatal(err)
	}
	monitorSource, err := os.ReadFile(filepath.Join("renop-html", "js", "backend-availability.js"))
	if err != nil {
		t.Fatal(err)
	}
	mainText := string(mainSource)
	monitorText := string(monitorSource)
	if strings.Contains(mainText, "offlineEl.style.display = 'flex'") {
		t.Fatal("main.js still exposes the offline overlay after a single fetch failure")
	}
	for _, required := range []string{
		"installBackendAvailabilityMonitor", "/api/status/health", "visibilitychange",
		"documentObject.visibilityState === 'visible'", "await this.delay(probeRetryDelayMs)",
	} {
		if !strings.Contains(mainText+monitorText, required) {
			t.Fatalf("backend availability monitor is missing %q", required)
		}
	}
	if strings.Count(monitorText, "await this.runProbe()") != 2 {
		t.Fatal("backend availability failures are not confirmed by exactly two probes")
	}
}

func TestSystemUpdatePromptsUseMessageCenter(t *testing.T) {
	mainSource, err := os.ReadFile(filepath.Join("renop-html", "js", "main.js"))
	if err != nil {
		t.Fatal(err)
	}
	dashboardSource, err := os.ReadFile(filepath.Join("renop-html", "js", "dashboard.js"))
	if err != nil {
		t.Fatal(err)
	}
	rendererSource, err := os.ReadFile(filepath.Join("renop-html", "js", "updater-messages.js"))
	if err != nil {
		t.Fatal(err)
	}
	mainText := string(mainSource)
	dashboardText := string(dashboardSource)
	rendererText := string(rendererSource)
	if !strings.Contains(mainText, "import './updater-messages.js'") ||
		!strings.Contains(rendererText, "registerMessageRenderer('system_update'") {
		t.Fatal("system update notifications are not registered with the message center")
	}
	for _, removedPrompt := range []string{
		"showUpdateModal(data)",
		"window.showAlert(t('dashboard.updateError'",
	} {
		if strings.Contains(dashboardText, removedPrompt) {
			t.Fatalf("dashboard retains automatic system update prompt %q", removedPrompt)
		}
	}
	if !strings.Contains(dashboardText, "showUpdateModal(updateInfo)") {
		t.Fatal("explicit dashboard update review control was removed")
	}
	for _, transientToast := range []string{
		"showAlert(t('dashboard.downloadingBg'), 'info')",
		"showAlert(t('dashboard.restarting'), 'info')",
	} {
		if !strings.Contains(dashboardText, transientToast) {
			t.Fatalf("transient update feedback is missing toast %q", transientToast)
		}
	}
	offlineSource, err := os.ReadFile(filepath.Join("renop-html", "js", "alert.js"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(offlineSource), "export function showOfflineUpdateModal") {
		t.Fatal("offline update dialog was changed or removed")
	}
	for _, required := range []string{
		"accept: '.br,.zip'", "isSupportedOfflineUpdate(file)",
		"updaterErrorMessage(result, 'updater.uploadFailed')", "showAlert(t('updater.uploadFailed'), 'error')",
	} {
		if !strings.Contains(string(offlineSource), required) {
			t.Fatalf("offline update dialog is missing Brotli/ZIP compatibility marker %q", required)
		}
	}
	if strings.Contains(string(offlineSource), "result.status ? `HTTP ${result.status}` : t('common.unknown')") {
		t.Fatal("offline update failures still fall back to an unknown or raw HTTP error")
	}
	updaterErrorSource, err := os.ReadFile(filepath.Join("renop-html", "js", "updater-errors.js"))
	if err != nil {
		t.Fatal(err)
	}
	updaterErrorText := string(updaterErrorSource)
	for _, required := range []string{
		"X-Renop-Error-Code", "invalid_package", "incompatible_binary", "package_too_large",
		"package_processing_failed", "restart_failed",
	} {
		if !strings.Contains(updaterErrorText, required) {
			t.Fatalf("localized updater error mapping is missing %q", required)
		}
	}
	if strings.Contains(updaterErrorText, "responseText") {
		t.Fatal("updater error mapping exposes raw response bodies")
	}
}

func TestTeamRemovalMessagesHideOperator(t *testing.T) {
	moduleSource, err := os.ReadFile(filepath.Join("renop-html", "js", "team-messages.js"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(moduleSource)
	for _, required := range []string{
		"registerMessageRenderer('package_team_removed'", "team.removedRepositoryBody", "team.removedMavenBody",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("team removal message renderer is missing %q", required)
		}
	}
	for _, forbidden := range []string{"message.sender", "payload.actor", "payload.operator", "payload.inviter"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("team removal message renderer exposes operator metadata through %q", forbidden)
		}
	}
	mainSource, err := os.ReadFile(filepath.Join("renop-html", "js", "main.js"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mainSource), "import './team-messages.js'") {
		t.Fatal("team removal message renderer is not loaded by the application")
	}
}

func TestRepositoryListUsesTypeIconsAndVisibilityDots(t *testing.T) {
	source, err := os.ReadFile(filepath.Join("renop-html", "js", "repositories.js"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	for _, required := range []string{
		"createIcon(format.icon || 'repositoryFiles')",
		"cfg-repository-visibility is-", "cfg-repository-delete-btn", "'aria-label': t('repos.deleteRepoTitle'",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("repository list visual metadata is missing %q", required)
		}
	}
	formatSource, err := os.ReadFile(filepath.Join("renop-html", "js", "repository-formats.js"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"icon: 'repositoryMaven'", "icon: 'repositoryCargo'",
		"icon: 'repositoryDocker'", "icon: 'repositoryFiles'",
	} {
		if !strings.Contains(string(formatSource), required) {
			t.Fatalf("repository format catalog is missing %q", required)
		}
	}
	for _, obsolete := range []string{"cfg-format-badge", "makeVisibilityBadge", "createIcon('delete'), el('span'"} {
		if strings.Contains(text, obsolete) {
			t.Fatalf("repository list retains obsolete text badge markup %q", obsolete)
		}
	}
	cssSource, err := os.ReadFile(filepath.Join("renop-html", "css", "manager", "settings.css"))
	if err != nil {
		t.Fatal(err)
	}
	cssText := string(cssSource)
	for _, required := range []string{
		".cfg-repository-visibility.is-public", ".cfg-repository-visibility.is-hidden",
		".cfg-repository-visibility.is-private", ".cfg-repository-type-icon.is-maven",
		".cfg-repository-type-icon.is-cargo", ".cfg-repository-type-icon.is-docker",
	} {
		if !strings.Contains(cssText, required) {
			t.Fatalf("repository list styles are missing %q", required)
		}
	}
}

func TestRepositoryEngineMigrationUIUsesRecoverableLocalizedAction(t *testing.T) {
	source, err := os.ReadFile(filepath.Join("renop-html", "js", "repositories.js"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	for _, required := range []string{
		"function buildRepositoryMigrationControl", "migrate/${target}", "runButtonAction(button",
		"repository_migration_pending_gpg", "repos.migrationPendingGpg", "repos.migrationSuccess",
		"await initRepositories()",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("repository migration UI is missing %q", required)
		}
	}
	start := strings.Index(text, "function repositoryMigrationErrorMessage")
	if start < 0 {
		t.Fatal("repository migration UI boundary is missing")
	}
	end := strings.Index(text[start:], "function buildRepoSection")
	if end < 0 {
		t.Fatal("repository migration UI boundary is missing")
	}
	if strings.Contains(text[start:start+end], "response.text()") {
		t.Fatal("repository migration UI exposes raw backend response text")
	}
	cssSource, err := os.ReadFile(filepath.Join("renop-html", "css", "manager", "settings.css"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{".repository-migration-control", ".repository-migration-button"} {
		if !strings.Contains(string(cssSource), required) {
			t.Fatalf("repository migration styling is missing %q", required)
		}
	}
}

func TestPackageViewsUseExplicitMirrorProvenance(t *testing.T) {
	for _, sourcePath := range []string{
		filepath.Join("renop-html", "js", "browser", "maven.js"),
		filepath.Join("renop-html", "js", "browser", "cargo.js"),
		filepath.Join("renop-html", "js", "browser", "docker.js"),
	} {
		source, err := os.ReadFile(sourcePath)
		if err != nil {
			t.Fatal(err)
		}
		text := string(source)
		if !strings.Contains(text, "createRepositoryMirrorBadge") || !strings.Contains(text, ".mirrored") {
			t.Fatalf("%s does not render explicit mirror provenance", sourcePath)
		}
		if strings.Contains(text, "push_enabled === false") {
			t.Fatalf("%s infers mirror provenance from an unrelated capability", sourcePath)
		}
	}
	sharedSource, err := os.ReadFile(filepath.Join("renop-html", "js", "browser", "repository-view.js"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(sharedSource), "export function createRepositoryMirrorBadge") {
		t.Fatal("repository views are missing the shared mirror provenance badge")
	}
}

func TestAccountMenuOwnsMessagesLogoutAndNotificationComposer(t *testing.T) {
	indexSource, err := os.ReadFile(filepath.Join("renop-html", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	indexText := string(indexSource)
	menuStart := strings.Index(indexText, `id="profile-menu"`)
	appStart := strings.Index(indexText, `id="app"`)
	if menuStart < 0 || appStart <= menuStart {
		t.Fatal("account menu boundary is missing")
	}
	accountMenu := indexText[menuStart:appStart]
	if !strings.Contains(indexText[:menuStart], `id="profile-message-unread-badge"`) {
		t.Fatal("account trigger is missing its unread-message badge")
	}
	for _, required := range []string{
		`id="message-center-btn"`, `data-account-action="messages"`, `id="logout-btn"`,
		`id="message-compose-menu-btn"`, `data-account-action="compose-notification"`,
	} {
		if !strings.Contains(accountMenu, required) {
			t.Fatalf("account menu is missing %q", required)
		}
	}
	composeModalStart := strings.Index(indexText, `id="message-compose-modal"`)
	messageModalStart := strings.Index(indexText, `id="message-center-modal"`)
	if messageModalStart < 0 || composeModalStart <= messageModalStart {
		t.Fatal("administrator notification composer is not an independent modal")
	}
	messageCenterMarkup := indexText[messageModalStart:composeModalStart]
	for _, obsolete := range []string{`id="message-compose-toggle"`, `id="message-compose-form"`} {
		if strings.Contains(messageCenterMarkup, obsolete) {
			t.Fatalf("message center still contains administrator composer markup %q", obsolete)
		}
	}
	messagesSource, err := os.ReadFile(filepath.Join("renop-html", "js", "messages.js"))
	if err != nil {
		t.Fatal(err)
	}
	messagesText := string(messagesSource)
	for _, required := range []string{
		"export function openNotificationComposer", "messages.length > 0 && nextCursor !== ''",
		"document.getElementById('profile-message-unread-badge')",
		"button.disabled = loading || !hasMore", "button.hidden = !hasMore",
	} {
		if !strings.Contains(messagesText, required) {
			t.Fatalf("message controls are missing %q", required)
		}
	}
	if strings.Contains(messagesText, "loadMore.disabled = false") {
		t.Fatal("message pagination still enables load-more without a server cursor")
	}
}

func TestNotificationComposerAndAccountMenuUseCompactStructuredLayout(t *testing.T) {
	indexSource, err := os.ReadFile(filepath.Join("renop-html", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	indexText := string(indexSource)
	composeStart := strings.Index(indexText, `id="message-compose-modal"`)
	loginStart := strings.Index(indexText, `id="login-modal"`)
	if composeStart < 0 || loginStart <= composeStart {
		t.Fatal("notification composer markup boundary is missing")
	}
	composer := indexText[composeStart:loginStart]
	for _, required := range []string{
		`class="message-compose-heading-icon"`,
		`class="message-compose-audience"`,
		`class="message-compose-meta"`,
		`class="modal-footer message-compose-actions"`,
	} {
		if !strings.Contains(composer, required) {
			t.Fatalf("notification composer is missing %q", required)
		}
	}
	composerCSS, err := os.ReadFile(filepath.Join("renop-html", "css", "components", "message-center.css"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"width: min(620px, calc(100vw - 2rem))",
		"max-height: min(88dvh, 760px)",
		".message-compose-audience",
		".message-compose-meta",
		"scrollbar-gutter: stable",
	} {
		if !strings.Contains(string(composerCSS), required) {
			t.Fatalf("notification composer styling is missing %q", required)
		}
	}
	navigationCSS, err := os.ReadFile(filepath.Join("renop-html", "css", "layout", "navigation.css"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"width: min(11.75rem, calc(100vw - 1rem))",
		"max-width: calc(100vw - max(1rem, env(safe-area-inset-left)) - max(1rem, env(safe-area-inset-right)))",
		"padding: 0.9rem max(1rem, env(safe-area-inset-right)) 0.55rem max(1rem, env(safe-area-inset-left))",
		"white-space: normal",
		".nav-profile-menu-item > span:not(.message-unread-badge)",
		".profile-message-unread-badge",
	} {
		if !strings.Contains(string(navigationCSS), required) {
			t.Fatalf("compact account menu styling is missing %q", required)
		}
	}
	if strings.Contains(string(navigationCSS), "right: -2.4rem") {
		t.Fatal("mobile account menu still extends beyond the viewport")
	}
}

func TestDynamicLocalizationAndMobileDialogViewportGuards(t *testing.T) {
	i18nSource, err := os.ReadFile(filepath.Join("renop-html", "js", "i18n.js"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"new MutationObserver",
		"record.addedNodes.forEach(queueTranslationRoot)",
		"attributeFilter: translationBindings.map",
		"translateSubtree(document)",
	} {
		if !strings.Contains(string(i18nSource), required) {
			t.Fatalf("dynamic localization is missing %q", required)
		}
	}
	modalCSS, err := os.ReadFile(filepath.Join("..", "..", "..", "packages", "renop-ui", "css", "components", "modal.css"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"height: 100dvh",
		"env(safe-area-inset-top)",
		"env(safe-area-inset-bottom)",
		"max-height: calc(100dvh",
		"overscroll-behavior: contain",
	} {
		if !strings.Contains(string(modalCSS), required) {
			t.Fatalf("mobile dialog viewport guard is missing %q", required)
		}
	}
	for _, stylesheet := range []string{
		filepath.Join("renop-html", "css", "components", "message-center.css"),
		filepath.Join("renop-html", "css", "manager", "settings.css"),
		filepath.Join("renop-html", "css", "manager", "users.css"),
	} {
		source, err := os.ReadFile(stylesheet)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(source), "dvh") {
			t.Fatalf("long dialog stylesheet %s lacks a dynamic viewport cap", stylesheet)
		}
	}
}

func TestFineGrainedAPITokenProfileUI(t *testing.T) {
	indexSource, err := os.ReadFile(filepath.Join("renop-html", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	indexText := string(indexSource)
	for _, required := range []string{
		`id="profile-api-token-section"`, `id="profile-api-token-status"`, `id="btn-manage-api-tokens"`,
		`id="profile-account-security-section" class="profile-settings-section profile-account-security-section profile-collapsible-card"`,
		`id="profile-private-email" class="profile-input" type="email" maxlength="254" autocomplete="email" required`,
	} {
		if !strings.Contains(indexText, required) {
			t.Fatalf("fine-grained API token profile UI is missing %q", required)
		}
	}
	if strings.Contains(indexText, `id="btn-generate-upload-token"`) {
		t.Fatal("profile still exposes the legacy single upload-token control")
	}
	source, err := os.ReadFile(filepath.Join("renop-html", "js", "api-tokens.js"))
	if err != nil {
		t.Fatal(err)
	}
	sourceText := string(source)
	for _, required := range []string{
		"/api/auth/profile/api-tokens", "expires_at", "repository:publish", "team:manage", "domain:verify", "runButtonAction",
		"writeClipboardText", "profile.apiTokenSecretWarning", "data-api-token-scope",
		"data-i18n-placeholder", "languageChanged", "makeCustomSelect", "profile-api-token-create-modal",
		"profile-api-token-scope-groups", "profile.apiTokenScopeGroup.${group.key}",
		"target_kinds", "target_limit", "data-api-token-target-for", "profile.apiTokenTargetsHint",
	} {
		if !strings.Contains(sourceText, required) {
			t.Fatalf("fine-grained API token controller is missing %q", required)
		}
	}
	profileCSS, err := os.ReadFile(filepath.Join("renop-html", "css", "manager", "profile.css"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		".profile-api-token-scope-grid", ".profile-api-token-secret", "overflow-x: auto",
		".profile-api-token-create-modal .modal-body", "scrollbar-gutter: stable",
		"padding: 1.1rem 1.35rem 1.35rem !important",
	} {
		if !strings.Contains(string(profileCSS), required) {
			t.Fatalf("fine-grained API token styling is missing %q", required)
		}
	}
	userRowSource, err := os.ReadFile(filepath.Join("renop-html", "js", "components", "user-row.js"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(userRowSource), "options.onReset") {
		t.Fatal("administrator user actions still expose obsolete token creation")
	}
	componentsSource, err := os.ReadFile(filepath.Join("renop-html", "css", "components.css"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(componentsSource), `@import "@renop/ui/css/components/custom-select.css";`) {
		t.Fatal("frontend does not import the canonical custom-select stylesheet")
	}
	fieldRowSource, err := os.ReadFile(filepath.Join("renop-html", "css", "components", "field-row.css"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(fieldRowSource), ".custom-select-dropdown") {
		t.Fatal("field-row stylesheet still duplicates the shared custom-select implementation")
	}
}

func TestSharedShellRoutingAvatarCodeAndSearchAnimations(t *testing.T) {
	indexSource, err := os.ReadFile(filepath.Join("renop-html", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(indexSource), `id="home-link" href="/"`) {
		t.Fatal("navigation title is missing the explicit home control")
	}
	mainSource, err := os.ReadFile(filepath.Join("renop-html", "js", "main.js"))
	if err != nil {
		t.Fatal(err)
	}
	mainText := string(mainSource)
	for _, required := range []string{"async function navigateHome()", "window.history.pushState(null, '', '/')", "await switchTab('overview')"} {
		if !strings.Contains(mainText, required) {
			t.Fatalf("home navigation is missing %q", required)
		}
	}
	navigationCSS, err := os.ReadFile(filepath.Join("renop-html", "css", "layout", "navigation.css"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"flex: 0 0 2.5rem", "min-width: 2.5rem", "aspect-ratio: 1"} {
		if !strings.Contains(string(navigationCSS), required) {
			t.Fatalf("navigation avatar sizing is missing %q", required)
		}
	}
	detailsCSS, err := os.ReadFile(filepath.Join("renop-html", "css", "browser", "details.css"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"overflow-x: auto", "width: max-content", "min-width: 100%"} {
		if !strings.Contains(string(detailsCSS), required) {
			t.Fatalf("copyable code viewport is missing %q", required)
		}
	}
	searchSource, err := os.ReadFile(filepath.Join("renop-html", "js", "browser", "search.js"))
	if err != nil {
		t.Fatal(err)
	}
	searchText := string(searchSource)
	for _, required := range []string{"morphElementHeight(panel, mutate", "function replaceResultsContent", "panel.style.bottom"} {
		if !strings.Contains(searchText, required) {
			t.Fatalf("repository search height animation is missing %q", required)
		}
	}
}

func TestMavenSearchUsesCatalogDomainResults(t *testing.T) {
	backendSource, err := os.ReadFile(filepath.Join("..", "..", "api", "search.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"repo.UsesModernMavenLayout()", "searchModernMavenRepository", "searchFileTreeRepository",
		`Path: "domains/" + domain.Domain`, `Type: "DOMAIN"`,
		`Path: "packages/" + artifact.GroupID + "/" + artifact.ArtifactID`,
	} {
		if !strings.Contains(string(backendSource), required) {
			t.Fatalf("format-aware Maven search is missing %q", required)
		}
	}
	frontendSource, err := os.ReadFile(filepath.Join("renop-html", "js", "browser", "search.js"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"normalized === 'DOMAIN'", "t('search.domain')", "type === 'DOMAIN'", "iconName = 'network'",
	} {
		if !strings.Contains(string(frontendSource), required) {
			t.Fatalf("Maven domain search presentation is missing %q", required)
		}
	}
}

func TestFrontendUsesSharedClipboardAndTimeUtilities(t *testing.T) {
	jsRoot := filepath.Join("renop-html", "js")
	clipboardPath := filepath.Join(jsRoot, "clipboard.js")
	clipboardSource, err := os.ReadFile(clipboardPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"navigator.clipboard", "document.execCommand('copy')", "textarea.remove()"} {
		if !strings.Contains(string(clipboardSource), required) {
			t.Fatalf("shared clipboard utility is missing %q", required)
		}
	}

	timestampPattern := regexp.MustCompile(`new Date\([^\r\n]*\)\.toLocale(?:Date|Time)?String\(`)
	err = filepath.WalkDir(jsRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".js" || strings.Contains(path, string(filepath.Separator)+"proto"+string(filepath.Separator)) {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		text := string(data)
		if path != clipboardPath && (strings.Contains(text, "navigator.clipboard") || strings.Contains(text, "execCommand('copy')")) {
			t.Fatalf("%s bypasses the shared clipboard utility", path)
		}
		if timestampPattern.MatchString(text) {
			t.Fatalf("%s contains duplicate timestamp normalization", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	timeSource, err := os.ReadFile(filepath.Join(jsRoot, "time.js"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"export function timestampMilliseconds", "export function formatTimestamp"} {
		if !strings.Contains(string(timeSource), required) {
			t.Fatalf("shared time utility is missing %q", required)
		}
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

	usedActions := make(map[string]struct{})
	for _, action := range audit.KnownActions() {
		if strings.HasPrefix(action, "DOCKER_") {
			usedActions[action] = struct{}{}
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
	if policy := indexResponse.Header.Get("Content-Security-Policy"); !strings.Contains(policy, "font-src 'self' https: http:") {
		t.Fatalf("index CSP does not permit validated asynchronous webfonts: %q", policy)
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
	cfg.LegalNoticeURL = `https://example.com/legal?a=1&b="notice"`

	generated := string(GenerateIndexHTMLFromConfig(&cfg))
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

	generated := string(GenerateIndexHTMLFromConfig(&cfg))
	if strings.Contains(generated, "{{RENOP.PUBLIC_SECURITY_FILING}}") {
		t.Fatal("generated HTML still contains the public security filing placeholder")
	}
	if !strings.Contains(generated, `京公网安备11000000000001号&lt;script&gt;`) {
		t.Fatal("generated HTML does not contain the escaped public security filing")
	}
}

func TestGenerateIndexHtmlIncludesSafeNonBlockingFontConfig(t *testing.T) {
	cfg := config.DefaultFrontendConfig()
	cfg.FontPreset = config.FrontendFontCustom
	cfg.FontURL = `https://fonts.example.com/interface.woff2?name="renop"&v=1`

	generated := string(GenerateIndexHTMLFromConfig(&cfg))
	if !strings.Contains(generated, `data-font-preset="custom"`) {
		t.Fatal("generated HTML does not activate the custom font preset")
	}
	if !strings.Contains(generated, `content="https://fonts.example.com/interface.woff2?name=&#34;renop&#34;&amp;v=1"`) {
		t.Fatal("generated HTML does not safely escape the custom font URL")
	}
	RefreshIndexHTMLCache(&cfg)
	if len(cfg.CachedIndexHTML) == 0 || !bytes.Equal(cfg.CachedIndexHTML, GenerateIndexHTMLFromConfig(&cfg)) {
		t.Fatal("frontend H5 cache was not refreshed deterministically")
	}
}
