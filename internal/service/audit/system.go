/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package audit

import (
	"errors"
	"fmt"
	"io"
	"log"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"renop/internal/core"

	"github.com/gofiber/fiber/v3"
)

var diagnosticCredentials = regexp.MustCompile(`(?i)((?:password|passwd|secret|client_secret|access_token|refresh_token|api_key|authorization|cookie|session_token|secret_access_key)["']?\s*[:=]\s*)(?:"[^"\r\n]*"|'[^'\r\n]*'|[^\s,;]+)`)
var diagnosticBearer = regexp.MustCompile(`(?i)\b(Bearer|Basic|Session)\s+[A-Za-z0-9_+/:.=-]+`)
var diagnosticURLCredentials = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.-]*://)[^\s/@]+@`)

func redactDiagnostic(message string) string {
	if len(message) > 16384 {
		message = message[:16384]
	}
	message = diagnosticBearer.ReplaceAllString(message, "${1} [redacted]")
	message = diagnosticCredentials.ReplaceAllString(message, "${1}[redacted]")
	message = diagnosticURLCredentials.ReplaceAllString(message, "${1}[redacted]@")
	return strings.ToValidUTF8(message, "")
}

type systemLogWriter struct {
	output  io.Writer
	state   *core.AppState
	dropped atomic.Uint64
}

func (w *systemLogWriter) Write(p []byte) (int, error) {
	n, err := w.output.Write(p)
	if len(p) > 16384 {
		p = p[:16384]
	}
	entry := &core.AuditLogEntry{
		Kind: "system", Action: ActionSystemLog, Trigger: "system", Operator: "system", Severity: "info",
		Details: redactDiagnostic(strings.TrimSpace(string(p))), CreatedAt: time.Now().UnixMilli(),
	}
	entry.Details = truncateDiagnostic(entry.Details)
	lower := strings.ToLower(entry.Details)
	if strings.Contains(lower, "failed") || strings.Contains(lower, "error") || strings.Contains(lower, "panic") {
		entry.Severity = "error"
	} else if strings.Contains(lower, "warning") {
		entry.Severity = "warning"
	}
	if dropped := w.dropped.Swap(0); dropped > 0 {
		notice := &core.AuditLogEntry{Kind: "system", Action: ActionSystemLogDropped, Trigger: "system",
			Operator: "system", Severity: "warning", CreatedAt: entry.CreatedAt,
			Details: fmt.Sprintf("%d system log records exceeded the queue capacity; consult the process log", dropped)}
		select {
		case w.state.Inner.AuditLogChan <- notice:
		default:
			w.dropped.Add(dropped)
		}
	}
	// Never persist synchronously from log.Writer: database errors can themselves log.
	select {
	case w.state.Inner.AuditLogChan <- entry:
	default:
		w.dropped.Add(1)
		w.state.Inner.FailuresCount.Add(1)
	}
	return n, err
}

func truncateDiagnostic(value string) string {
	if len(value) > 4096 {
		return strings.ToValidUTF8(value[:4096], "")
	}
	return value
}

func persistenceDiagnostic(format string, args ...any) {
	output := log.Writer()
	if capture, ok := output.(*systemLogWriter); ok {
		output = capture.output
	}
	log.New(output, log.Prefix(), log.Flags()).Printf(format, args...)
}

func logHTTPFailure(c fiber.Ctx, state *core.AppState, details string) {
	username, operator, method, session, ip := ExtractAuthDetails(c, state)
	Log(state, &core.AuditLogEntry{Kind: "system", Severity: "error", Trigger: "http",
		Username: username, Operator: operator, Action: ActionSystemHTTPError, Details: details,
		AuthMethod: method, SessionID: session, IP: ip})
}

// HTTPDiagnostics records server failures returned directly by route handlers.
func HTTPDiagnostics(state *core.AppState) fiber.Handler {
	return func(c fiber.Ctx) error {
		err := c.Next()
		if err == nil && c.Response().StatusCode() >= 500 {
			logHTTPFailure(c, state, fmt.Sprintf("%s %s returned HTTP %d", c.Method(), c.Path(), c.Response().StatusCode()))
		}
		return err
	}
}

// ErrorHandler records unhandled server errors while keeping diagnostic details out of public responses.
func ErrorHandler(state *core.AppState) fiber.ErrorHandler {
	return func(c fiber.Ctx, err error) error {
		status := fiber.StatusInternalServerError
		if httpError, ok := errors.AsType[*fiber.Error](err); ok {
			status = httpError.Code
		}
		if status < 500 {
			return fiber.DefaultErrorHandler(c, err)
		}
		logHTTPFailure(c, state, c.Method()+" "+c.Path()+": "+err.Error())
		return c.SendStatus(status)
	}
}
