/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package auth

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/database"
	"renop/internal/service/token"
)

func TestUserProfileRoutesValidateAndRateLimitRenames(t *testing.T) {
	state := core.NewAppState()
	cfg := config.DefaultConfig()
	cfg.Maven.Repositories["profile-maven"] = &config.Repository{
		Name: "profile-maven", Format: config.RepositoryFormatMaven, Visibility: "PUBLIC",
	}
	cfg.Maven.Repositories["profile-cargo"] = &config.Repository{
		Name: "profile-cargo", Format: config.RepositoryFormatCargo, Visibility: "PUBLIC",
	}
	cfg.Maven.Repositories["profile-docker"] = &config.Repository{
		Name: "profile-docker", Format: config.RepositoryFormatDocker, Visibility: "PUBLIC",
	}
	cfg.Maven.Repositories["profile-npm"] = &config.Repository{
		Name: "profile-npm", Format: config.RepositoryFormatNPM, Visibility: "PUBLIC",
	}
	cfg.Maven.Repositories["private-cargo"] = &config.Repository{
		Name: "private-cargo", Format: config.RepositoryFormatCargo, Visibility: "PRIVATE",
	}
	cfg.Maven.Repositories["hidden-cargo"] = &config.Repository{
		Name: "hidden-cargo", Format: config.RepositoryFormatCargo, Visibility: "HIDDEN",
	}
	cfg.Server.GitHubOAuth = config.GitHubOAuthConfig{
		Enabled: true, ClientID: "profile-client", ClientSecret: "profile-secret",
		CallbackURL: "https://repo.example/api/auth/github/callback",
	}
	state.Inner.Config.Store(cfg)
	db, err := database.InitDB(config.DatabaseConfig{
		Driver: "sqlite", Dsn: filepath.Join(t.TempDir(), "profile-routes.db"), MaxOpenConns: 1, MaxIdleConns: 1,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	state.Inner.DB = db
	for index, username := range []string{"alice", "bobby"} {
		passwordHash := ""
		if username == "alice" {
			passwordHash = "configured-password-hash"
		}
		require.NoError(t, db.SaveToken(&core.AccessToken{
			Identifier: core.AccessTokenIdentifier{Type: core.Persistent, Value: int32(index + 1)},
			Name:       username, EncryptedSecret: passwordHash,
			CreatedAt: "2026-08-24T00:00:00Z", Permissions: []string{"base"},
		}))
	}

	operations := make(chan token.TokenOp, 8)
	go token.StartTokenConsumer(state, operations)
	t.Cleanup(func() { close(operations) })

	currentUsername := "alice"
	currentRoles := []string{"base"}
	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		c.Locals("user", &config.User{Username: currentUsername, Roles: currentRoles})
		return c.Next()
	})
	SetupAuthRoutes(app, state, operations)

	bobbyProfile, err := db.GetUserProfile("bobby")
	require.NoError(t, err)
	aliceProfile, err := db.GetUserProfile("alice")
	require.NoError(t, err)
	const githubAuthorizedAt int64 = 1_800_000_000_000
	require.NoError(t, db.StoreGitHubIdentity(aliceProfile.UserID, 101, "alice-github", []core.GitHubPrincipal{
		{Type: core.GitHubPrincipalUser, GitHubID: 101, Login: "alice-github", AuthorizedAt: githubAuthorizedAt},
		{Type: core.GitHubPrincipalOrganization, GitHubID: 202, Login: "example-org", AuthorizedAt: githubAuthorizedAt},
	}, githubAuthorizedAt))
	const membershipCreatedAt int64 = 1_800_000_000_000
	require.NoError(t, db.RecordCargoPublication(&core.CargoPackage{
		Repository: "profile-cargo", Name: "profile-crate", NormalizedName: "profile-crate",
		Description: "Profile crate", CreatedAt: membershipCreatedAt, UpdatedAt: membershipCreatedAt,
	}, &core.CargoVersion{
		Repository: "profile-cargo", Package: "profile-crate", Version: "1.0.0",
		Publisher: "bobby", CreatedAt: membershipCreatedAt,
	}, "bobby"))
	require.NoError(t, db.RecordCargoPublication(&core.CargoPackage{
		Repository: "private-cargo", Name: "private-crate", NormalizedName: "private-crate",
		Description: "Private profile crate", CreatedAt: membershipCreatedAt, UpdatedAt: membershipCreatedAt,
	}, &core.CargoVersion{
		Repository: "private-cargo", Package: "private-crate", Version: "1.0.0",
		Publisher: "bobby", CreatedAt: membershipCreatedAt,
	}, "bobby"))
	require.NoError(t, db.RecordCargoPublication(&core.CargoPackage{
		Repository: "hidden-cargo", Name: "hidden-crate", NormalizedName: "hidden-crate",
		Description: "Unlisted profile crate", CreatedAt: membershipCreatedAt, UpdatedAt: membershipCreatedAt,
	}, &core.CargoVersion{
		Repository: "hidden-cargo", Package: "hidden-crate", Version: "1.0.0",
		Publisher: "bobby", CreatedAt: membershipCreatedAt,
	}, "bobby"))
	_, err = db.CreateDockerImage("profile-docker", "profile/image", "bobby", false, membershipCreatedAt)
	require.NoError(t, err)
	require.NoError(t, db.PutDockerManifest(&core.DockerManifest{
		Repository: "profile-docker", ImageName: "profile/image",
		Digest:    "sha256:abc1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcded",
		MediaType: "application/vnd.oci.image.manifest.v1+json", RawJSON: []byte(`{"schemaVersion":2}`),
	}, "latest", "bobby"))
	_, err = db.CreateNPMPackage("profile-npm", "@profile/library", "bobby", false, membershipCreatedAt)
	require.NoError(t, err)
	mavenDomain := &core.MavenDomain{
		Repository: "profile-maven", Domain: "com.example", VerificationType: core.MavenVerificationDNS,
		VerificationHost: "example.com", VerificationCode: "renop-verification=profile", CreatedAt: membershipCreatedAt,
	}
	require.NoError(t, db.CreateMavenDomain(mavenDomain, "bobby"))
	require.NoError(t, db.MarkMavenDomainVerified("com.example", mavenDomain.VerificationCode, membershipCreatedAt))

	response := profileRequest(t, app, http.MethodGet, "/users/bobby/profile", "")
	require.Equal(t, http.StatusOK, response.StatusCode)
	var publicProfile userProfileResponse
	require.NoError(t, json.NewDecoder(response.Body).Decode(&publicProfile))
	require.Equal(t, "bobby", publicProfile.Username)
	require.Equal(t, bobbyProfile.UserID, publicProfile.UserID)
	require.False(t, publicProfile.OwnProfile)
	require.Equal(t, 1, publicProfile.MavenDomainCount)
	require.Equal(t, 1, publicProfile.CargoPackageCount)
	require.Equal(t, 1, publicProfile.DockerImageCount)
	require.Equal(t, 1, publicProfile.NPMPackageCount)
	require.Nil(t, publicProfile.GitHub)
	require.Nil(t, publicProfile.SuperTeamLimits)
	require.Nil(t, publicProfile.PublicationQuota)
	require.NoError(t, response.Body.Close())

	response = profileRequest(t, app, http.MethodGet, "/users/alice/profile", "")
	require.Equal(t, http.StatusOK, response.StatusCode)
	var ownProfile userProfileResponse
	require.NoError(t, json.NewDecoder(response.Body).Decode(&ownProfile))
	require.NoError(t, response.Body.Close())
	require.True(t, ownProfile.OwnProfile)
	require.NotNil(t, ownProfile.GitHub)
	require.True(t, ownProfile.GitHub.Configured)
	require.True(t, ownProfile.GitHub.Linked)
	require.True(t, ownProfile.GitHub.CanDisconnect)
	require.Equal(t, "alice-github", ownProfile.GitHub.GitHubLogin)
	require.Equal(t, 2, ownProfile.GitHub.PrincipalCount)
	require.NotNil(t, ownProfile.SuperTeamLimits)
	require.Equal(t, cfg.SuperTeams.CreateLimit, ownProfile.SuperTeamLimits.CreateLimit)
	require.Equal(t, cfg.SuperTeams.JoinLimit, ownProfile.SuperTeamLimits.JoinLimit)
	require.NotNil(t, ownProfile.PublicationQuota)
	require.Equal(t, cfg.PublicationQuota.FileLimit, ownProfile.PublicationQuota.FileLimit)
	require.Equal(t, cfg.PublicationQuota.ByteLimit, ownProfile.PublicationQuota.ByteLimit)
	response = profileRequest(t, app, http.MethodGet, "/users/bobby/memberships?format=cargo", "")
	require.Equal(t, http.StatusOK, response.StatusCode)
	var membershipResponse struct {
		Memberships []*core.UserPackageMembership `json:"memberships"`
	}
	require.NoError(t, json.NewDecoder(response.Body).Decode(&membershipResponse))
	require.NoError(t, response.Body.Close())
	require.Len(t, membershipResponse.Memberships, 1)
	require.Equal(t, "profile-crate", membershipResponse.Memberships[0].Name)
	require.Equal(t, core.CargoPermissionOwner, membershipResponse.Memberships[0].PermissionLevel)
	response = profileRequest(t, app, http.MethodGet, "/users/bobby/memberships?format=maven", "")
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.NoError(t, json.NewDecoder(response.Body).Decode(&membershipResponse))
	require.NoError(t, response.Body.Close())
	require.Len(t, membershipResponse.Memberships, 1)
	require.Equal(t, "com.example", membershipResponse.Memberships[0].Name)
	require.Equal(t, core.MavenPermissionOwner, membershipResponse.Memberships[0].PermissionLevel)
	response = profileRequest(t, app, http.MethodGet, "/users/bobby/memberships?format=npm", "")
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.NoError(t, json.NewDecoder(response.Body).Decode(&membershipResponse))
	require.NoError(t, response.Body.Close())
	require.Len(t, membershipResponse.Memberships, 1)
	require.Equal(t, "@profile/library", membershipResponse.Memberships[0].Name)
	require.Equal(t, core.NPMPermissionOwner, membershipResponse.Memberships[0].PermissionLevel)
	currentUsername = "bobby"
	response = profileRequest(t, app, http.MethodGet, "/users/bobby/memberships?format=cargo", "")
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.NoError(t, json.NewDecoder(response.Body).Decode(&membershipResponse))
	require.NoError(t, response.Body.Close())
	require.Len(t, membershipResponse.Memberships, 2)
	currentUsername = "alice"
	currentRoles = []string{"base", "manager"}
	response = profileRequest(t, app, http.MethodGet, "/users/bobby/memberships?format=cargo", "")
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.NoError(t, json.NewDecoder(response.Body).Decode(&membershipResponse))
	require.NoError(t, response.Body.Close())
	require.Len(t, membershipResponse.Memberships, 3)
	require.Equal(t, "hidden-crate", membershipResponse.Memberships[0].Name)
	currentRoles = []string{"base"}
	response = profileRequest(t, app, http.MethodGet, "/users/profiles?names=alice,bobby,missing", "")
	require.Equal(t, http.StatusOK, response.StatusCode)
	var batch struct {
		Profiles []userProfileResponse `json:"profiles"`
	}
	require.NoError(t, json.NewDecoder(response.Body).Decode(&batch))
	require.NoError(t, response.Body.Close())
	require.Len(t, batch.Profiles, 2)
	for _, profile := range batch.Profiles {
		require.Nil(t, profile.GitHub)
	}

	response = profileRequest(t, app, http.MethodPut, "/auth/profile", `{"username":"bad-name"}`)
	require.Equal(t, http.StatusBadRequest, response.StatusCode)
	require.NoError(t, response.Body.Close())
	response = profileRequest(t, app, http.MethodPut, "/auth/profile",
		`{"nickname":"1234567890123456789012345678901234567"}`)
	require.Equal(t, http.StatusBadRequest, response.StatusCode)
	require.NoError(t, response.Body.Close())
	response = profileRequest(t, app, http.MethodPut, "/auth/profile", `{"username":"bobby"}`)
	require.Equal(t, http.StatusConflict, response.StatusCode)
	require.NoError(t, response.Body.Close())

	response = profileRequest(t, app, http.MethodPut, "/auth/profile",
		`{"username":"alice_one","nickname":"Alice Example"}`)
	require.Equal(t, http.StatusOK, response.StatusCode)
	var firstRename userProfileResponse
	require.NoError(t, json.NewDecoder(response.Body).Decode(&firstRename))
	require.NoError(t, response.Body.Close())
	require.Equal(t, "alice_one", firstRename.Username)
	require.Equal(t, "Alice Example", firstRename.Nickname)
	require.Equal(t, 1, firstRename.UsernameChangesRemaining)
	require.NotNil(t, firstRename.GitHub)
	require.Equal(t, "alice-github", firstRename.GitHub.GitHubLogin)
	require.NotNil(t, firstRename.SuperTeamLimits)
	currentUsername = firstRename.Username

	response = profileRequest(t, app, http.MethodPut, "/auth/profile", `{"username":"alice_two"}`)
	require.Equal(t, http.StatusOK, response.StatusCode)
	var secondRename userProfileResponse
	require.NoError(t, json.NewDecoder(response.Body).Decode(&secondRename))
	require.NoError(t, response.Body.Close())
	require.Equal(t, 0, secondRename.UsernameChangesRemaining)
	currentUsername = secondRename.Username

	response = profileRequest(t, app, http.MethodPut, "/auth/profile", `{"username":"alice_three"}`)
	require.Equal(t, http.StatusTooManyRequests, response.StatusCode)
	require.NotEmpty(t, response.Header.Get("Retry-After"))
	require.NoError(t, response.Body.Close())

	response = profileRequest(t, app, http.MethodGet, "/users/alice_two/profile", "")
	require.Equal(t, http.StatusOK, response.StatusCode)
	var renamedProfile userProfileResponse
	require.NoError(t, json.NewDecoder(response.Body).Decode(&renamedProfile))
	require.NoError(t, response.Body.Close())
	require.Equal(t, "alice_two", renamedProfile.Username)
	require.Equal(t, aliceProfile.UserID, renamedProfile.UserID)
	response = profileRequest(t, app, http.MethodGet, "/users/alice/profile", "")
	require.Equal(t, http.StatusNotFound, response.StatusCode)
	require.NoError(t, response.Body.Close())
}

func profileRequest(t *testing.T, app *fiber.App, method, path, body string) *http.Response {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	}
	response, err := app.Test(request)
	require.NoError(t, err)
	return response
}
