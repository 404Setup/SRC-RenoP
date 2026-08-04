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
	"bytes"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"

	"renop/config"
	"renop/core"
	"renop/pb"
	"renop/token"
	"renop/utils"
	"renop/utils/protohttp"
)

type FidoUser struct {
	Username    string
	ID          []byte
	Credentials []webauthn.Credential
}

func (u *FidoUser) WebAuthnID() []byte {
	return u.ID
}

func (u *FidoUser) WebAuthnName() string {
	return u.Username
}

func (u *FidoUser) WebAuthnDisplayName() string {
	return u.Username
}

func (u *FidoUser) WebAuthnCredentials() []webauthn.Credential {
	return u.Credentials
}

func buildFidoUser(username string, state *core.AppState) *FidoUser {
	devs := state.ListFidoDevices(username)
	creds := make([]webauthn.Credential, 0, len(devs))
	for _, d := range devs {
		creds = append(creds, webauthn.Credential{
			ID:              d.CredentialID,
			PublicKey:       d.PublicKey,
			AttestationType: d.AttestationType,
			Flags: webauthn.CredentialFlags{
				UserPresent:    d.UserPresent,
				UserVerified:   d.UserVerified,
				BackupEligible: d.BackupEligible,
				BackupState:    d.BackupState,
			},
			Authenticator: webauthn.Authenticator{
				AAGUID:    d.AAGUID,
				SignCount: d.SignCount,
			},
		})
	}
	return &FidoUser{
		Username:    username,
		ID:          []byte(strings.ToLower(username)),
		Credentials: creds,
	}
}

type fidoSessionEntry struct {
	sessionData *webauthn.SessionData
	username    string
	createdAt   time.Time
}

var (
	fidoSessionMap sync.Map
	cleanFidoOnce  sync.Once
)

func storeFidoSession(sessionID string, sessionData *webauthn.SessionData, username string) {
	fidoSessionMap.Store(sessionID, &fidoSessionEntry{
		sessionData: sessionData,
		username:    username,
		createdAt:   time.Now(),
	})
	cleanFidoOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(5 * time.Minute)
			for range ticker.C {
				now := time.Now()
				fidoSessionMap.Range(func(key, val any) bool {
					if entry, ok := val.(*fidoSessionEntry); ok {
						if now.Sub(entry.createdAt) > 10*time.Minute {
							fidoSessionMap.Delete(key)
						}
					}
					return true
				})
			}
		}()
	})
}

func getFidoSession(sessionID string) (*webauthn.SessionData, string, bool) {
	val, ok := fidoSessionMap.LoadAndDelete(sessionID)
	if !ok {
		return nil, "", false
	}
	entry := val.(*fidoSessionEntry)
	if time.Since(entry.createdAt) > 10*time.Minute {
		return nil, "", false
	}
	return entry.sessionData, entry.username, true
}

func getWebAuthnEngine(c fiber.Ctx, state *core.AppState) (*webauthn.WebAuthn, error) {
	var cfg *config.Config
	if state != nil {
		if cfgVal := state.Inner.Config.Load(); cfgVal != nil {
			cfg = cfgVal.(*config.Config)
		}
	}

	originHeader := strings.TrimSpace(c.Get("Origin"))
	refererHeader := strings.TrimSpace(c.Get("Referer"))
	fwdHost := strings.TrimSpace(c.Get("X-Forwarded-Host"))
	if before, _, ok := strings.Cut(fwdHost, ","); ok {
		fwdHost = strings.TrimSpace(before)
	}

	scheme := "http"
	if isSecure(c) {
		scheme = "https"
	}

	var host string
	if originHeader != "" {
		if u, err := url.Parse(originHeader); err == nil && u.Host != "" {
			scheme = u.Scheme
			host = u.Host
		}
	} else if refererHeader != "" {
		if u, err := url.Parse(refererHeader); err == nil && u.Host != "" {
			scheme = u.Scheme
			host = u.Host
		}
	}

	if host == "" {
		if fwdHost != "" {
			host = fwdHost
		} else {
			host = string(c.Request().Header.Host())
		}
	}
	if host == "" {
		host = "localhost"
	}

	hostname := host
	port := ""
	if h, p, err := net.SplitHostPort(host); err == nil {
		hostname = h
		port = p
	} else {
		hostname = strings.TrimPrefix(strings.TrimSuffix(hostname, "]"), "[")
	}

	rpID := hostname
	if rpID == "0.0.0.0" || rpID == "" {
		if cfg != nil && len(cfg.Server.Domains) > 0 && cfg.Server.PrimaryDomain() != "" && cfg.Server.PrimaryDomain() != "0.0.0.0" {
			rpID = cfg.Server.PrimaryDomain()
		} else {
			rpID = "localhost"
		}
	}

	originMap := make(map[string]struct{})
	addOrigin := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		if !strings.Contains(s, "://") {
			return
		}
		if u, err := url.Parse(s); err == nil && u.Scheme != "" && u.Host != "" {
			normalized := strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host)
			originMap[normalized] = struct{}{}
		}
	}

	primaryOrigin := scheme + "://" + host
	addOrigin(primaryOrigin)

	if originHeader != "" {
		addOrigin(originHeader)
	}
	if refererHeader != "" {
		addOrigin(refererHeader)
	}

	addOrigin("http://" + rpID)
	addOrigin("https://" + rpID)
	if port != "" {
		addOrigin("http://" + rpID + ":" + port)
		addOrigin("https://" + rpID + ":" + port)
	}

	if cfg != nil {
		for _, d := range cfg.Server.Domains {
			addOrigin("http://" + d)
			addOrigin("https://" + d)
			if cfg.Server.Port > 0 {
				addOrigin("http://" + d + ":" + strconv.Itoa(int(cfg.Server.Port)))
				addOrigin("https://" + d + ":" + strconv.Itoa(int(cfg.Server.Port)))
			}
		}
		for _, o := range cfg.Server.CorsOrigins {
			if strings.Contains(o, "://") && !strings.Contains(o, "*") {
				addOrigin(o)
			}
		}
	}

	devHosts := []string{"localhost", "127.0.0.1", "[::1]"}
	devPorts := []string{"3000", "8080", "80", "443"}
	if cfg != nil && cfg.Server.Port > 0 {
		devPorts = append(devPorts, strconv.Itoa(int(cfg.Server.Port)))
	}
	for _, dh := range devHosts {
		addOrigin("http://" + dh)
		addOrigin("https://" + dh)
		for _, dp := range devPorts {
			addOrigin("http://" + dh + ":" + dp)
			addOrigin("https://" + dh + ":" + dp)
		}
	}

	rpOrigins := make([]string, 0, len(originMap))
	for o := range originMap {
		rpOrigins = append(rpOrigins, o)
	}

	return webauthn.New(&webauthn.Config{
		RPDisplayName: "Renop",
		RPID:          rpID,
		RPOrigins:     rpOrigins,
	})
}

func SetupFidoRoutes(app fiber.Router, state *core.AppState) {
	fido := app.Group("/fido")
	fido.Post("/login/begin", func(c fiber.Ctx) error { return PostFidoLoginBegin(c, state) })
	fido.Post("/login/finish", func(c fiber.Ctx) error { return PostFidoLoginFinish(c, state) })

	profileFido := app.Group("/profile/fido")
	profileFido.Get("", func(c fiber.Ctx) error { return GetProfileFidoDevices(c, state) })
	profileFido.Post("/register/begin", func(c fiber.Ctx) error { return PostFidoRegisterBegin(c, state) })
	profileFido.Post("/register/finish", func(c fiber.Ctx) error { return PostFidoRegisterFinish(c, state) })
	profileFido.Delete("/:device_id", func(c fiber.Ctx) error { return DeleteProfileFidoDevice(c, state) })

	usersFido := app.Group("/users/:username/fido")
	usersFido.Get("", RequireManager, func(c fiber.Ctx) error { return GetUserFidoDevices(c, state) })
	usersFido.Delete("/:device_id", RequireManager, func(c fiber.Ctx) error { return DeleteUserFidoDevice(c, state) })
}

type FidoLoginBeginRequest struct {
	Username string `json:"username"`
}

type FidoRegisterFinishRequest struct {
	SessionID  string          `json:"session_id"`
	Name       string          `json:"name"`
	Credential json.RawMessage `json:"credential"`
}

type FidoLoginFinishRequest struct {
	SessionID  string          `json:"session_id"`
	Credential json.RawMessage `json:"credential"`
}

func PostFidoRegisterBegin(c fiber.Ctx, state *core.AppState) error {
	userInt := c.Locals("user")
	if userInt == nil {
		return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized")
	}
	user := userInt.(*config.User)

	w, err := getWebAuthnEngine(c, state)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("WebAuthn initialization failed")
	}

	fidoUser := buildFidoUser(user.Username, state)
	options, sessionData, err := w.BeginRegistration(
		fidoUser,
		webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{
			AuthenticatorAttachment: "",
			ResidentKey:             protocol.ResidentKeyRequirementPreferred,
			UserVerification:        protocol.VerificationPreferred,
		}),
		webauthn.WithConveyancePreference(protocol.PreferNoAttestation),
	)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).SendString("Failed to begin registration: " + err.Error())
	}

	sessionID := uuid.NewString()
	storeFidoSession(sessionID, sessionData, user.Username)

	return c.JSON(fiber.Map{
		"session_id": sessionID,
		"options":    options,
	})
}

func PostFidoRegisterFinish(c fiber.Ctx, state *core.AppState) error {
	userInt := c.Locals("user")
	if userInt == nil {
		return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized")
	}
	user := userInt.(*config.User)

	var req FidoRegisterFinishRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).SendString("Bad Request")
	}

	sessionData, sessionUser, ok := getFidoSession(req.SessionID)
	if !ok || !strings.EqualFold(sessionUser, user.Username) {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid or expired session")
	}

	w, err := getWebAuthnEngine(c, state)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("WebAuthn initialization failed")
	}

	fidoUser := buildFidoUser(user.Username, state)
	httpReq, err := http.NewRequest("POST", "", bytes.NewReader(req.Credential))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid credential payload")
	}
	httpReq.Header.Set("Content-Type", "application/json")

	parsedResponse, err := protocol.ParseCredentialCreationResponse(httpReq)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).SendString("Failed to parse creation response: " + err.Error())
	}

	credential, err := w.CreateCredential(fidoUser, *sessionData, parsedResponse)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).SendString("Registration failed: " + err.Error())
	}

	deviceName := strings.TrimSpace(req.Name)
	if deviceName == "" {
		deviceName = "FIDO Device"
	}

	device := &core.FidoDevice{
		ID:              uuid.NewString(),
		Username:        user.Username,
		Name:            deviceName,
		CredentialID:    credential.ID,
		PublicKey:       credential.PublicKey,
		AttestationType: credential.AttestationType,
		AAGUID:          credential.Authenticator.AAGUID,
		SignCount:       credential.Authenticator.SignCount,
		CreatedAt:       time.Now().UnixMilli(),
		UserPresent:     credential.Flags.UserPresent,
		UserVerified:    credential.Flags.UserVerified,
		BackupEligible:  credential.Flags.BackupEligible,
		BackupState:     credential.Flags.BackupState,
	}

	state.SaveFidoDevice(device)

	return c.JSON(fiber.Map{
		"status": "success",
		"device": core.FidoDeviceDto{
			ID:        device.ID,
			Username:  device.Username,
			Name:      device.Name,
			CreatedAt: device.CreatedAt,
		},
	})
}

func GetProfileFidoDevices(c fiber.Ctx, state *core.AppState) error {
	userInt := c.Locals("user")
	if userInt == nil {
		return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized")
	}
	user := userInt.(*config.User)

	devs := state.ListFidoDevices(user.Username)
	dtos := make([]core.FidoDeviceDto, 0, len(devs))
	for _, d := range devs {
		dtos = append(dtos, core.FidoDeviceDto{
			ID:        d.ID,
			Username:  d.Username,
			Name:      d.Name,
			CreatedAt: d.CreatedAt,
		})
	}
	return protohttp.Write(c, pb.FromFidoDeviceList(dtos))
}

func DeleteProfileFidoDevice(c fiber.Ctx, state *core.AppState) error {
	userInt := c.Locals("user")
	if userInt == nil {
		return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized")
	}
	user := userInt.(*config.User)
	deviceID := c.Params("device_id")

	state.DeleteFidoDevice(user.Username, deviceID)
	return protohttp.Write(c, pb.StatusOkSuccess())
}

func GetUserFidoDevices(c fiber.Ctx, state *core.AppState) error {
	username := c.Params("username")
	devs := state.ListFidoDevices(username)
	dtos := make([]core.FidoDeviceDto, 0, len(devs))
	for _, d := range devs {
		dtos = append(dtos, core.FidoDeviceDto{
			ID:        d.ID,
			Username:  d.Username,
			Name:      d.Name,
			CreatedAt: d.CreatedAt,
		})
	}
	return protohttp.Write(c, pb.FromFidoDeviceList(dtos))
}

func DeleteUserFidoDevice(c fiber.Ctx, state *core.AppState) error {
	username := c.Params("username")
	deviceID := c.Params("device_id")

	state.DeleteFidoDevice(username, deviceID)
	return protohttp.Write(c, pb.StatusOkSuccess())
}

func PostFidoLoginBegin(c fiber.Ctx, state *core.AppState) error {
	var req FidoLoginBeginRequest
	_ = c.Bind().JSON(&req)

	w, err := getWebAuthnEngine(c, state)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("WebAuthn initialization failed")
	}

	reqUsername := strings.TrimSpace(req.Username)
	var options *protocol.CredentialAssertion
	var sessionData *webauthn.SessionData

	userVerificationOpt := webauthn.WithUserVerification(protocol.VerificationPreferred)

	if reqUsername != "" {
		fidoUser := buildFidoUser(reqUsername, state)
		if len(fidoUser.Credentials) > 0 {
			options, sessionData, err = w.BeginLogin(fidoUser, userVerificationOpt)
		} else {
			options, sessionData, err = w.BeginDiscoverableLogin(userVerificationOpt)
		}
	} else {
		options, sessionData, err = w.BeginDiscoverableLogin(userVerificationOpt)
	}

	if err != nil {
		return c.Status(fiber.StatusBadRequest).SendString("Failed to begin FIDO login: " + err.Error())
	}

	sessionID := uuid.NewString()
	storeFidoSession(sessionID, sessionData, reqUsername)

	return c.JSON(fiber.Map{
		"session_id": sessionID,
		"options":    options,
	})
}

func PostFidoLoginFinish(c fiber.Ctx, state *core.AppState) error {
	cfgVal := state.Inner.Config.Load()
	cfg := cfgVal.(*config.Config)
	ip := utils.ExtractIP(c, &cfg.Server)
	userAgent := c.Get(fiber.HeaderUserAgent, "Unknown")

	var req FidoLoginFinishRequest
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).SendString("Bad Request")
	}

	sessionData, sessionUsername, ok := getFidoSession(req.SessionID)
	if !ok {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid or expired login session")
	}

	w, err := getWebAuthnEngine(c, state)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("WebAuthn initialization failed")
	}

	httpReq, err := http.NewRequest("POST", "", bytes.NewReader(req.Credential))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid credential payload")
	}
	httpReq.Header.Set("Content-Type", "application/json")

	parsedResponse, err := protocol.ParseCredentialRequestResponse(httpReq)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).SendString("Failed to parse assertion response: " + err.Error())
	}

	incomingBE := parsedResponse.Response.AuthenticatorData.Flags.HasBackupEligible()
	if incomingBE {
		if matchedDevice := state.GetFidoDeviceByCredentialID(parsedResponse.RawID); matchedDevice != nil && !matchedDevice.BackupEligible {
			matchedDevice.BackupEligible = true
			state.UpdateFidoDeviceState(matchedDevice.CredentialID, matchedDevice.SignCount, matchedDevice.BackupState, true)
		}
	}

	var authenticatedUser *config.User
	var matchedCred *webauthn.Credential

	userHandler := func(rawID, userHandle []byte) (webauthn.User, error) {
		matchedDevice := state.GetFidoDeviceByCredentialID(rawID)
		if matchedDevice == nil {
			return nil, errors.New("FIDO credential not found")
		}
		targetUsername := matchedDevice.Username
		if len(userHandle) > 0 {
			expectedHandle := strings.ToLower(targetUsername)
			if !strings.EqualFold(string(userHandle), expectedHandle) && !bytes.Equal(userHandle, []byte(expectedHandle)) {
				return nil, errors.New("FIDO user handle mismatch")
			}
		}
		accessToken := state.GetTokenByName(targetUsername)
		if accessToken == nil && state.Inner.TokensCount.Load() == 0 {
			token.AutoRegisterAdmin(state, nil)
			accessToken = state.GetTokenByName(targetUsername)
		}
		if accessToken != nil {
			authenticatedUser = buildSynthUser(accessToken)
		}
		return buildFidoUser(targetUsername, state), nil
	}

	var lastErr error
	if sessionUsername != "" {
		fidoUser := buildFidoUser(sessionUsername, state)
		if len(fidoUser.Credentials) > 0 {
			var loginErr error
			matchedCred, loginErr = w.ValidateLogin(fidoUser, *sessionData, parsedResponse)
			if loginErr == nil && matchedCred != nil {
				accessToken := state.GetTokenByName(sessionUsername)
				if accessToken == nil && state.Inner.TokensCount.Load() == 0 {
					token.AutoRegisterAdmin(state, nil)
					accessToken = state.GetTokenByName(sessionUsername)
				}
				if accessToken != nil {
					authenticatedUser = buildSynthUser(accessToken)
				}
			} else if loginErr != nil {
				lastErr = loginErr
			}
		}
	}

	if matchedCred == nil || authenticatedUser == nil {
		var discErr error
		matchedCred, discErr = w.ValidateDiscoverableLogin(userHandler, *sessionData, parsedResponse)
		if discErr != nil && lastErr == nil {
			lastErr = discErr
		}
	}

	if matchedCred != nil && authenticatedUser == nil {
		matchedDevice := state.GetFidoDeviceByCredentialID(matchedCred.ID)
		if matchedDevice != nil {
			accessToken := state.GetTokenByName(matchedDevice.Username)
			if accessToken == nil && state.Inner.TokensCount.Load() == 0 {
				token.AutoRegisterAdmin(state, nil)
				accessToken = state.GetTokenByName(matchedDevice.Username)
			}
			if accessToken != nil {
				authenticatedUser = buildSynthUser(accessToken)
			}
		}
	}

	if authenticatedUser == nil || matchedCred == nil {
		errMsg := "FIDO authentication failed"
		if lastErr != nil {
			errMsg += ": " + lastErr.Error()
		}
		return c.Status(fiber.StatusUnauthorized).SendString(errMsg)
	}

	state.UpdateFidoDeviceState(matchedCred.ID, matchedCred.Authenticator.SignCount, matchedCred.Flags.BackupState, matchedCred.Flags.BackupEligible)

	sessionToken := uuid.NewString()
	publicId := uuid.NewString()
	now := time.Now().UnixMilli()

	session := &core.Session{
		PublicId:  publicId,
		Username:  authenticatedUser.Username,
		Ip:        utils.Intern(ip),
		UserAgent: utils.Intern(userAgent),
		CreatedAt: now,
	}
	session.LastActive.Store(now)

	state.SaveSession(session, sessionToken)

	setSessionCookie(c, sessionToken, int(core.SessionIdleTimeoutMillis/1000))
	details := CreateSessionDetails(authenticatedUser, "")
	if strings.Contains(c.Get(fiber.HeaderAccept), protohttp.ContentType) {
		return protohttp.Write(c, pb.FromSessionDetails(details))
	}
	return c.JSON(details)
}
