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
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"golang.org/x/image/webp"

	"renop/internal/config"
	"renop/internal/core"
	"renop/internal/service/audit"
	"renop/internal/service/publicationquota"
	"renop/internal/utils"
)

const (
	minAvatarDimension = 256
	maxAvatarDimension = 1000
)

var (
	errAvatarType       = errors.New("avatar format is not supported")
	errAvatarTooLarge   = errors.New("avatar exceeds the size limit")
	errAvatarDimensions = errors.New("avatar dimensions are invalid")
	errAvatarUnsafe     = errors.New("avatar container is invalid")
)

type avatarWriter struct {
	buffer bytes.Buffer
	limit  int64
}

func (writer *avatarWriter) Write(value []byte) (int, error) {
	if int64(writer.buffer.Len())+int64(len(value)) > writer.limit {
		return 0, errAvatarTooLarge
	}
	return writer.buffer.Write(value)
}

func validatePNGContainer(value []byte) error {
	if len(value) < 20 || !bytes.Equal(value[:8], []byte("\x89PNG\r\n\x1a\n")) {
		return errAvatarType
	}
	for offset := 8; ; {
		if offset+12 > len(value) {
			return errAvatarUnsafe
		}
		length := int64(binary.BigEndian.Uint32(value[offset : offset+4]))
		if length > int64(len(value)-offset-12) {
			return errAvatarUnsafe
		}
		end := offset + 12 + int(length)
		chunkType := value[offset+4 : offset+8]
		checksum := binary.BigEndian.Uint32(value[end-4 : end])
		if crc32.ChecksumIEEE(value[offset+4:end-4]) != checksum {
			return errAvatarUnsafe
		}
		if bytes.Equal(chunkType, []byte("IEND")) {
			if length != 0 || end != len(value) {
				return errAvatarUnsafe
			}
			return nil
		}
		offset = end
	}
}

func avatarFormat(value []byte) (string, error) {
	switch {
	case len(value) >= 8 && bytes.Equal(value[:8], []byte("\x89PNG\r\n\x1a\n")):
		if err := validatePNGContainer(value); err != nil {
			return "", err
		}
		return "png", nil
	case len(value) >= 4 && value[0] == 0xff && value[1] == 0xd8:
		if value[len(value)-2] != 0xff || value[len(value)-1] != 0xd9 {
			return "", errAvatarUnsafe
		}
		return "jpeg", nil
	case len(value) >= 12 && bytes.Equal(value[:4], []byte("RIFF")) && bytes.Equal(value[8:12], []byte("WEBP")):
		if int64(binary.LittleEndian.Uint32(value[4:8]))+8 != int64(len(value)) {
			return "", errAvatarUnsafe
		}
		return "webp", nil
	default:
		return "", errAvatarType
	}
}

func avatarDeclaredFormat(contentType string) string {
	contentType, _, _ = mime.ParseMediaType(contentType)
	switch strings.ToLower(strings.TrimSpace(contentType)) {
	case "image/png":
		return "png"
	case "image/jpeg", "image/jpg":
		return "jpeg"
	case "image/webp":
		return "webp"
	default:
		return ""
	}
}

func decodeAvatarConfig(format string, source io.Reader) (image.Config, error) {
	switch format {
	case "png":
		return png.DecodeConfig(source)
	case "jpeg":
		return jpeg.DecodeConfig(source)
	case "webp":
		return webp.DecodeConfig(source)
	default:
		return image.Config{}, errAvatarType
	}
}

func decodeAvatar(format string, source io.Reader) (image.Image, error) {
	switch format {
	case "png":
		return png.Decode(source)
	case "jpeg":
		return jpeg.Decode(source)
	case "webp":
		return webp.Decode(source)
	default:
		return nil, errAvatarType
	}
}

func validAvatarDimensions(width, height int) bool {
	return width == height && width >= minAvatarDimension && width <= maxAvatarDimension
}

func normalizeAvatar(value []byte, declaredType string, maxSize int64) (*core.UserAvatar, error) {
	if maxSize < int64(config.MinAvatarMaxSizeBytes) || maxSize > int64(config.MaxAvatarMaxSizeBytes) {
		maxSize = int64(config.DefaultAvatarMaxSizeBytes)
	}
	if len(value) == 0 || int64(len(value)) > maxSize {
		return nil, errAvatarTooLarge
	}
	format, err := avatarFormat(value)
	if err != nil {
		return nil, err
	}
	if declared := avatarDeclaredFormat(declaredType); declared == "" || declared != format {
		return nil, errAvatarType
	}
	decodedConfig, err := decodeAvatarConfig(format, bytes.NewReader(value))
	if err != nil {
		return nil, errAvatarUnsafe
	}
	if !validAvatarDimensions(decodedConfig.Width, decodedConfig.Height) {
		return nil, errAvatarDimensions
	}
	decoded, err := decodeAvatar(format, bytes.NewReader(value))
	if err != nil {
		return nil, errAvatarUnsafe
	}
	if !validAvatarDimensions(decoded.Bounds().Dx(), decoded.Bounds().Dy()) {
		return nil, errAvatarDimensions
	}
	writer := &avatarWriter{limit: maxSize}
	contentType := "image/png"
	if format == "jpeg" {
		contentType = "image/jpeg"
		err = jpeg.Encode(writer, decoded, &jpeg.Options{Quality: 90})
	} else {
		err = png.Encode(writer, decoded)
	}
	if err != nil {
		if errors.Is(err, errAvatarTooLarge) {
			return nil, errAvatarTooLarge
		}
		return nil, errAvatarUnsafe
	}
	data := writer.buffer.Bytes()
	sum := sha256.Sum256(data)
	return &core.UserAvatar{
		ContentType: contentType, Data: data, Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:]),
	}, nil
}

func avatarSizeLimit(state *core.AppState) int64 {
	if state == nil || state.Inner == nil || state.Inner.Config.Load() == nil {
		return int64(config.DefaultAvatarMaxSizeBytes)
	}
	value := state.Inner.Config.Load().Server.AvatarMaxSizeBytes
	if value < config.MinAvatarMaxSizeBytes || value > config.MaxAvatarMaxSizeBytes {
		value = config.DefaultAvatarMaxSizeBytes
	}
	return int64(value)
}

func avatarAPIError(c fiber.Ctx, status int, code string) error {
	c.Set("X-Renop-Error-Code", code)
	return c.Status(status).SendString(code)
}

func avatarValidationError(c fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, errAvatarTooLarge):
		return avatarAPIError(c, fiber.StatusRequestEntityTooLarge, "avatar_too_large")
	case errors.Is(err, errAvatarType):
		return avatarAPIError(c, fiber.StatusBadRequest, "avatar_invalid_type")
	case errors.Is(err, errAvatarDimensions):
		return avatarAPIError(c, fiber.StatusBadRequest, "avatar_dimensions")
	default:
		return avatarAPIError(c, fiber.StatusBadRequest, "avatar_unsafe")
	}
}

func avatarProfileResponse(c fiber.Ctx, state *core.AppState, username string) error {
	profile, err := state.GetDB().GetUserProfile(username)
	if err != nil {
		return avatarAPIError(c, fiber.StatusInternalServerError, "avatar_profile_failed")
	}
	response, err := profileResponseWithPrivateDetails(state, profile, true, true, time.Now().UnixMilli())
	if err != nil {
		return avatarAPIError(c, fiber.StatusInternalServerError, "avatar_profile_failed")
	}
	response.AdministratorView = GetUser(c).IsManager()
	c.Set(fiber.HeaderCacheControl, "no-store")
	return c.JSON(response)
}

func storeAvatar(c fiber.Ctx, state *core.AppState, username, source string, avatar *core.UserAvatar) error {
	quota, err := publicationquota.Reserve(state, username, "", core.PublicationQuotaDelta{
		Files: 1, Bytes: avatar.Size, Publications: 0,
	})
	if err != nil {
		code := publicationquota.ErrorCode(err)
		return avatarAPIError(c, fiber.StatusTooManyRequests, code)
	}
	defer quota.Release()
	if err := quota.Commit(); err != nil {
		return avatarAPIError(c, fiber.StatusServiceUnavailable, "publication_quota_unavailable")
	}
	avatar.UpdatedAt = time.Now().UnixMilli()
	if err := state.GetDB().PutUserAvatar(username, avatar); err != nil {
		return avatarAPIError(c, fiber.StatusInternalServerError, "avatar_storage_failed")
	}
	_, operator, authMethod, sessionID, ip := audit.ExtractAuthDetails(c, state)
	audit.Log(state, &core.AuditLogEntry{
		Username: username, Operator: operator, Action: audit.ActionProfileUpdate,
		Details: "Updated profile avatar from " + source, AuthMethod: authMethod, SessionID: sessionID,
		IP: ip, CreatedAt: avatar.UpdatedAt,
	})
	return avatarProfileResponse(c, state, username)
}

func putOwnAvatar(c fiber.Ctx, state *core.AppState) error {
	user := GetUser(c)
	if user == nil || user.Username == "" || strings.EqualFold(user.Username, "guest") {
		return avatarAPIError(c, fiber.StatusUnauthorized, "authentication_required")
	}
	limit := avatarSizeLimit(state)
	body, err := utils.ReadRequestBodyLimited(c, limit)
	if err != nil {
		if errors.Is(err, fiber.ErrRequestEntityTooLarge) {
			return avatarValidationError(c, errAvatarTooLarge)
		}
		return avatarValidationError(c, errAvatarUnsafe)
	}
	avatar, err := normalizeAvatar(body, c.Get(fiber.HeaderContentType), limit)
	if err != nil {
		return avatarValidationError(c, err)
	}
	return storeAvatar(c, state, user.Username, "upload", avatar)
}

func deleteOwnAvatar(c fiber.Ctx, state *core.AppState) error {
	user := GetUser(c)
	if user == nil || user.Username == "" || strings.EqualFold(user.Username, "guest") {
		return avatarAPIError(c, fiber.StatusUnauthorized, "authentication_required")
	}
	if err := state.GetDB().DeleteUserAvatar(user.Username); err != nil {
		return avatarAPIError(c, fiber.StatusInternalServerError, "avatar_storage_failed")
	}
	_, operator, authMethod, sessionID, ip := audit.ExtractAuthDetails(c, state)
	audit.Log(state, &core.AuditLogEntry{
		Username: user.Username, Operator: operator, Action: audit.ActionProfileUpdate,
		Details: "Removed profile avatar", AuthMethod: authMethod, SessionID: sessionID,
		IP: ip, CreatedAt: time.Now().UnixMilli(),
	})
	return avatarProfileResponse(c, state, user.Username)
}

func getPublicAvatar(c fiber.Ctx, state *core.AppState) error {
	avatar, err := state.GetDB().GetUserAvatar(c.Params("username"))
	if errors.Is(err, core.ErrUserAvatarNotFound) || errors.Is(err, core.ErrUserProfileNotFound) {
		return c.SendStatus(fiber.StatusNotFound)
	}
	if err != nil {
		return avatarAPIError(c, fiber.StatusInternalServerError, "avatar_storage_failed")
	}
	etag := `"` + avatar.SHA256 + `"`
	c.Set(fiber.HeaderETag, etag)
	c.Set(fiber.HeaderXContentTypeOptions, "nosniff")
	c.Set(fiber.HeaderContentType, avatar.ContentType)
	c.Set(fiber.HeaderContentDisposition, `inline; filename="avatar`+map[string]string{
		"image/png": ".png", "image/jpeg": ".jpg",
	}[avatar.ContentType]+`"`)
	if strings.EqualFold(strings.TrimSpace(c.Query("v")), avatar.SHA256) {
		c.Set(fiber.HeaderCacheControl, "public, max-age=31536000, immutable")
	} else {
		c.Set(fiber.HeaderCacheControl, "public, max-age=300, must-revalidate")
	}
	if strings.TrimSpace(c.Get(fiber.HeaderIfNoneMatch)) == etag {
		return c.SendStatus(fiber.StatusNotModified)
	}
	return c.Send(avatar.Data)
}

func fetchGitHubAvatar(ctx context.Context, state *core.AppState, provider githubOAuthProvider,
	githubUserID int64, maxSize int64,
) ([]byte, string, error) {
	if githubUserID <= 0 || strings.TrimSpace(provider.AvatarURL) == "" {
		return nil, "", errors.New("GitHub avatar endpoint is unavailable")
	}
	client, err := githubOAuthHTTPClient(state.Inner.Config.Load())
	if err != nil {
		return nil, "", err
	}
	endpoint := strings.TrimRight(provider.AvatarURL, "/") + "/" + strconv.FormatInt(githubUserID, 10) + "?s=1000&v=4"
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, "", err
	}
	request.Header.Set("Accept", "image/png,image/jpeg,image/webp")
	request.Header.Set("User-Agent", "RenoP-GitHub-Avatar/1")
	response, err := client.Do(request)
	if err != nil {
		return nil, "", err
	}
	defer utils.DiscardHTTPBody(response.Body, response.ContentLength)
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, "", fmt.Errorf("GitHub avatar returned HTTP %d", response.StatusCode)
	}
	if response.ContentLength > maxSize {
		return nil, "", errAvatarTooLarge
	}
	value, err := utils.ReadAllLimited(response.Body, maxSize)
	if err != nil {
		if errors.Is(err, utils.ErrResponseTooLarge) {
			return nil, "", errAvatarTooLarge
		}
		return nil, "", err
	}
	return value, response.Header.Get(fiber.HeaderContentType), nil
}

func syncGitHubAvatar(c fiber.Ctx, state *core.AppState, provider githubOAuthProvider) error {
	user := GetUser(c)
	if user == nil || user.Username == "" || strings.EqualFold(user.Username, "guest") {
		return avatarAPIError(c, fiber.StatusUnauthorized, "authentication_required")
	}
	identity, err := state.GetDB().GetGitHubIdentity(user.Username)
	if err != nil || identity == nil || identity.GitHubUserID <= 0 {
		return avatarAPIError(c, fiber.StatusConflict, "github_not_linked")
	}
	limit := avatarSizeLimit(state)
	value, contentType, err := fetchGitHubAvatar(c.Context(), state, provider, identity.GitHubUserID, limit)
	if err != nil {
		if errors.Is(err, errAvatarTooLarge) {
			return avatarValidationError(c, err)
		}
		return avatarAPIError(c, fiber.StatusBadGateway, "avatar_download_failed")
	}
	avatar, err := normalizeAvatar(value, contentType, limit)
	if err != nil {
		return avatarValidationError(c, err)
	}
	return storeAvatar(c, state, user.Username, "GitHub", avatar)
}

func setupAvatarRoutes(auth fiber.Router, state *core.AppState) {
	auth.Put("/profile/avatar", func(c fiber.Ctx) error { return putOwnAvatar(c, state) })
	auth.Delete("/profile/avatar", func(c fiber.Ctx) error { return deleteOwnAvatar(c, state) })
}
