/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

// Package message exposes the user message center and a small delivery API for
// feature modules. Action execution remains in the owning feature; messages
// carry typed payloads, never client-controlled callback URLs.
package message

import (
	"bytes"
	"encoding/base64"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"

	"renop/internal/core"
	"renop/internal/service/audit"
	"renop/internal/service/auth"
	"renop/internal/utils/protohttp"
	"renop/pkg/pb"
)

const (
	defaultPageSize    = 30
	maxPageSize        = 100
	maxRequestSize     = 16 << 10
	maxRecipients      = 100
	maxUserSuggestions = 8
)

type listResponse struct {
	Messages    []*core.UserMessage `json:"messages"`
	UnreadCount int                 `json:"unread_count"`
	NextCursor  string              `json:"next_cursor,omitempty"`
}

type notificationRequest struct {
	Recipients []string `json:"recipients"`
	All        bool     `json:"all"`
	Severity   string   `json:"severity"`
	Title      string   `json:"title"`
	Body       string   `json:"body"`
}

type userSearchResponse struct {
	Users []string `json:"users"`
}

func SetupRoutes(router fiber.Router, state *core.AppState) {
	router.Get("/messages", func(c fiber.Ctx) error { return list(c, state) })
	router.Get("/messages/unread-count", func(c fiber.Ctx) error { return unreadCount(c, state) })
	router.Post("/messages/read-all", func(c fiber.Ctx) error { return markAllRead(c, state) })
	router.Post("/messages/:id/read", func(c fiber.Ctx) error { return markRead(c, state) })
	router.Delete("/messages", func(c fiber.Ctx) error { return clear(c, state) })
	router.Delete("/messages/:id", func(c fiber.Ctx) error { return remove(c, state) })
	router.Get("/messages/admin/users", func(c fiber.Ctx) error { return searchUsers(c, state) })
	router.Post("/messages/admin", func(c fiber.Ctx) error { return sendNotification(c, state) })
}

// Deliver persists one feature-generated message.
func Deliver(state *core.AppState, userMessage *core.UserMessage) error {
	return DeliverBatch(state, []*core.UserMessage{userMessage})
}

// DeliverBatch persists messages atomically and fills server-owned IDs and
// timestamps when the producer leaves them empty.
func DeliverBatch(state *core.AppState, messages []*core.UserMessage) error {
	if state == nil || len(messages) == 0 {
		return nil
	}
	now := time.Now().UnixMilli()
	for _, userMessage := range messages {
		if userMessage == nil {
			return errors.New("message is nil")
		}
		if userMessage.ID == "" {
			userMessage.ID = uuid.NewString()
		}
		if userMessage.CreatedAt == 0 {
			userMessage.CreatedAt = now
		}
	}
	db := state.GetDB()
	if db == nil {
		return core.ErrDatabaseUnavailable
	}
	return db.SaveMessages(messages)
}

func authenticatedUsername(c fiber.Ctx) (string, error) {
	user := auth.GetUser(c)
	if user == nil || user.Username == "" || user.Username == "guest" {
		return "", fiber.ErrUnauthorized
	}
	return strings.ToLower(user.Username), nil
}

func list(c fiber.Ctx, state *core.AppState) error {
	username, err := authenticatedUsername(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized")
	}
	limit, err := strconv.Atoi(c.Query("limit", strconv.Itoa(defaultPageSize)))
	if err != nil || limit < 1 || limit > maxPageSize {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid message limit")
	}
	beforeAt, beforeID, err := decodeCursor(c.Query("cursor"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid message cursor")
	}
	db := state.GetDB()
	if db == nil {
		return c.Status(fiber.StatusServiceUnavailable).SendString("Message center is unavailable")
	}
	now := time.Now().UnixMilli()
	messages, err := db.ListMessages(username, limit, beforeAt, beforeID, now)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to list messages")
	}
	unread, err := db.CountUnreadMessages(username, now)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to count messages")
	}
	nextCursor := ""
	if len(messages) == limit {
		last := messages[len(messages)-1]
		nextCursor = encodeCursor(last.CreatedAt, last.ID)
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	return protohttp.Write(c, pb.FromUserMessageList(messages, unread, nextCursor))
}

func unreadCount(c fiber.Ctx, state *core.AppState) error {
	username, err := authenticatedUsername(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized")
	}
	db := state.GetDB()
	if db == nil {
		return c.Status(fiber.StatusServiceUnavailable).SendString("Message center is unavailable")
	}
	count, err := db.CountUnreadMessages(username, time.Now().UnixMilli())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to count messages")
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	return protohttp.Write(c, pb.FromUnreadCount(count))
}

func markRead(c fiber.Ctx, state *core.AppState) error {
	username, err := authenticatedUsername(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized")
	}
	id := c.Params("id")
	if uuid.Validate(id) != nil {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid message ID")
	}
	db := state.GetDB()
	if db == nil {
		return c.Status(fiber.StatusServiceUnavailable).SendString("Message center is unavailable")
	}
	changed, err := db.MarkMessageRead(id, username, time.Now().UnixMilli())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to update message")
	}
	if !changed {
		message, lookupErr := db.GetUserMessage(id, username, time.Now().UnixMilli())
		if lookupErr != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to update message")
		}
		if message == nil {
			return c.Status(fiber.StatusNotFound).SendString("Message not found")
		}
	}
	return protohttp.Write(c, pb.StatusOkSuccess())
}

func markAllRead(c fiber.Ctx, state *core.AppState) error {
	username, err := authenticatedUsername(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized")
	}
	db := state.GetDB()
	if db == nil {
		return c.Status(fiber.StatusServiceUnavailable).SendString("Message center is unavailable")
	}
	updated, err := db.MarkAllMessagesRead(username, time.Now().UnixMilli())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to update messages")
	}
	return protohttp.Write(c, pb.FromMarkAllRead(updated))
}

func clear(c fiber.Ctx, state *core.AppState) error {
	username, err := authenticatedUsername(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized")
	}
	db := state.GetDB()
	if db == nil {
		return c.Status(fiber.StatusServiceUnavailable).SendString("Message center is unavailable")
	}
	deleted, err := db.DeleteUserMessages(username)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to clear messages")
	}
	return protohttp.Write(c, pb.FromClearMessages(deleted))
}

func remove(c fiber.Ctx, state *core.AppState) error {
	username, err := authenticatedUsername(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).SendString("Unauthorized")
	}
	id := c.Params("id")
	if uuid.Validate(id) != nil {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid message ID")
	}
	db := state.GetDB()
	if db == nil {
		return c.Status(fiber.StatusServiceUnavailable).SendString("Message center is unavailable")
	}
	deleted, err := db.DeleteUserMessage(id, username)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to delete message")
	}
	if !deleted {
		if message, lookupErr := db.GetUserMessage(id, username, time.Now().UnixMilli()); lookupErr != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to delete message")
		} else if message != nil && message.ActionStatus == core.MessageActionPending {
			return c.Status(fiber.StatusConflict).SendString("Pending action messages cannot be deleted")
		}
		return c.Status(fiber.StatusNotFound).SendString("Message not found")
	}
	return protohttp.Write(c, pb.StatusOkSuccess())
}

func sendNotification(c fiber.Ctx, state *core.AppState) error {
	sender := auth.GetUser(c)
	if sender == nil || !sender.IsManager() {
		return c.Status(fiber.StatusForbidden).SendString("Forbidden")
	}
	var protoReq pb.SendNotificationRequest
	readErr := protohttp.Read(c, &protoReq)
	if readErr == fiber.ErrRequestEntityTooLarge {
		return readErr
	}
	var request notificationRequest
	if readErr == nil && (len(protoReq.Recipients) > 0 || protoReq.All || protoReq.Title != "" || protoReq.Body != "") {
		request.Recipients = protoReq.Recipients
		request.All = protoReq.All
		request.Severity = protoReq.Severity
		request.Title = protoReq.Title
		request.Body = protoReq.Body
	} else {
		if err := decodeJSONRequest(c, &request); err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid notification")
		}
	}
	request.Title = strings.TrimSpace(request.Title)
	request.Body = strings.TrimSpace(request.Body)
	request.Severity = strings.ToLower(strings.TrimSpace(request.Severity))
	if request.Severity == "" {
		request.Severity = "info"
	}
	if !validMessageText(request.Title, 240, false) || !validMessageText(request.Body, 8000, true) {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid notification content")
	}
	switch request.Severity {
	case "info", "success", "warning", "error":
	default:
		return c.Status(fiber.StatusBadRequest).SendString("Invalid notification severity")
	}
	recipients, err := resolveRecipients(state, request)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).SendString(err.Error())
	}
	now := time.Now().UnixMilli()
	messages := make([]*core.UserMessage, 0, len(recipients))
	for _, recipient := range recipients {
		messages = append(messages, &core.UserMessage{
			Recipient: recipient,
			Sender:    strings.ToLower(sender.Username),
			Kind:      "announcement",
			Severity:  request.Severity,
			Title:     request.Title,
			Body:      request.Body,
			Payload:   []byte("{}"),
			CreatedAt: now,
		})
	}
	if err := DeliverBatch(state, messages); err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to send notification")
	}
	user, operator, authMethod, sessionID, ip := audit.ExtractAuthDetails(c, state)
	audit.Log(state, &core.AuditLogEntry{
		Username: user, Operator: operator, Action: "MESSAGE_SEND",
		Details:    "Sent a notification to " + strconv.Itoa(len(messages)) + " user(s)",
		AuthMethod: authMethod, SessionID: sessionID, IP: ip,
	})
	return protohttp.WriteStatus(c, fiber.StatusCreated, pb.FromSendNotification(int64(len(messages))))
}

func searchUsers(c fiber.Ctx, state *core.AppState) error {
	manager := auth.GetUser(c)
	if manager == nil || !manager.IsManager() {
		return c.Status(fiber.StatusForbidden).SendString("Forbidden")
	}
	c.Set(fiber.HeaderCacheControl, "no-store")
	query := strings.ToLower(strings.TrimSpace(c.Query("q")))
	if query == "" {
		return protohttp.Write(c, pb.FromUserSearch([]string{}))
	}
	if !validMessageText(query, 255, false) {
		return c.Status(fiber.StatusBadRequest).SendString("Invalid user search")
	}
	db := state.GetDB()
	if db == nil {
		return c.Status(fiber.StatusServiceUnavailable).SendString("Message center is unavailable")
	}
	users, err := db.SearchTokenNames(query, maxUserSuggestions, time.Now().UnixMilli())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).SendString("Failed to search users")
	}
	return protohttp.Write(c, pb.FromUserSearch(users))
}

func resolveRecipients(state *core.AppState, request notificationRequest) ([]string, error) {
	unique := make(map[string]struct{})
	if request.All {
		for _, token := range state.GetAllTokens() {
			if token == nil {
				continue
			}
			name := strings.ToLower(strings.TrimSpace(token.Name))
			if name != "" {
				unique[name] = struct{}{}
			}
		}
	} else {
		if len(request.Recipients) == 0 || len(request.Recipients) > maxRecipients {
			return nil, errors.New("choose between 1 and 100 recipients")
		}
		for _, rawName := range request.Recipients {
			name := strings.ToLower(strings.TrimSpace(rawName))
			if name == "" || len(name) > 255 || state.GetTokenByName(name) == nil {
				return nil, errors.New("notification recipient does not exist")
			}
			unique[name] = struct{}{}
		}
	}
	if len(unique) == 0 {
		return nil, errors.New("no notification recipients were found")
	}
	recipients := make([]string, 0, len(unique))
	for name := range unique {
		recipients = append(recipients, name)
	}
	return recipients, nil
}

func decodeJSONRequest(c fiber.Ctx, destination any) error {
	var reader io.Reader
	if stream := c.Request().BodyStream(); stream != nil {
		reader = stream
	} else {
		reader = bytes.NewReader(c.Request().Body())
	}
	limited := &io.LimitedReader{R: reader, N: maxRequestSize + 1}
	decoder := json.NewDecoder(limited)
	decoder.DisallowUnknownFields()
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

func validMessageText(value string, limit int, allowNewline bool) bool {
	if value == "" || len(value) > limit || !utf8.ValidString(value) {
		return false
	}
	for _, char := range value {
		if unicode.IsControl(char) && !(allowNewline && (char == '\n' || char == '\r' || char == '\t')) {
			return false
		}
	}
	return true
}

func encodeCursor(createdAt int64, id string) string {
	value := strconv.FormatInt(createdAt, 10) + ":" + id
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}

func decodeCursor(cursor string) (int64, string, error) {
	if cursor == "" {
		return 0, "", nil
	}
	if len(cursor) > 160 {
		return 0, "", errors.New("cursor is too long")
	}
	decoded, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return 0, "", err
	}
	timestampText, id, ok := strings.Cut(string(decoded), ":")
	if !ok || uuid.Validate(id) != nil {
		return 0, "", errors.New("invalid cursor")
	}
	timestamp, err := strconv.ParseInt(timestampText, 10, 64)
	if err != nil || timestamp <= 0 {
		return 0, "", errors.New("invalid cursor timestamp")
	}
	return timestamp, id, nil
}
