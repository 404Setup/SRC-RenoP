/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package database_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

var (
	deleteFromPattern = regexp.MustCompile(`(?i)\bDELETE\s+FROM\b`)
	wherePattern      = regexp.MustCompile(`(?i)\bWHERE\b`)
)

func flattenSQLStringExpression(expression ast.Expr, query *strings.Builder) bool {
	switch value := expression.(type) {
	case *ast.BasicLit:
		if value.Kind != token.STRING {
			return false
		}
		decoded, err := strconv.Unquote(value.Value)
		if err != nil {
			return false
		}
		query.WriteString(decoded)
		return true
	case *ast.BinaryExpr:
		if value.Op != token.ADD {
			return false
		}
		left := flattenSQLStringExpression(value.X, query)
		query.WriteByte(' ')
		right := flattenSQLStringExpression(value.Y, query)
		return left || right
	case *ast.ParenExpr:
		return flattenSQLStringExpression(value.X, query)
	default:
		query.WriteByte(' ')
		return false
	}
}

func unboundedDeleteStatements(query string) []string {
	matches := deleteFromPattern.FindAllStringIndex(query, -1)
	unsafe := make([]string, 0, len(matches))
	for _, match := range matches {
		end := len(query)
		if separator := strings.IndexByte(query[match[0]:], ';'); separator >= 0 {
			end = match[0] + separator
		}
		statement := strings.TrimSpace(query[match[0]:end])
		if !wherePattern.MatchString(statement) {
			unsafe = append(unsafe, statement)
		}
	}
	return unsafe
}

func isStringPrefixProbe(node ast.Node) bool {
	call, ok := node.(*ast.CallExpr)
	if !ok {
		return false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "HasPrefix" {
		return false
	}
	qualifier, ok := selector.X.(*ast.Ident)
	return ok && qualifier.Name == "strings"
}

func TestDatabaseDeletesAlwaysHaveWhereClause(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate database safety test")
	}
	repositoryRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", ".."))
	err := filepath.WalkDir(repositoryRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "dist", "node_modules":
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		fileSet := token.NewFileSet()
		parsed, parseErr := parser.ParseFile(fileSet, path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		stack := make([]ast.Node, 0, 16)
		ast.Inspect(parsed, func(node ast.Node) bool {
			if node == nil {
				stack = stack[:len(stack)-1]
				return false
			}
			var parent ast.Node
			if len(stack) > 0 {
				parent = stack[len(stack)-1]
			}
			stack = append(stack, node)

			expression, isExpression := node.(ast.Expr)
			if !isExpression {
				return true
			}
			if isStringPrefixProbe(parent) {
				return true
			}
			if parentBinary, nested := parent.(*ast.BinaryExpr); nested && parentBinary.Op == token.ADD {
				return true
			}
			switch value := expression.(type) {
			case *ast.BasicLit:
				if value.Kind != token.STRING {
					return true
				}
			case *ast.BinaryExpr:
				if value.Op != token.ADD {
					return true
				}
			default:
				return true
			}

			var query strings.Builder
			if !flattenSQLStringExpression(expression, &query) {
				return true
			}
			for _, statement := range unboundedDeleteStatements(query.String()) {
				position := fileSet.Position(expression.Pos())
				relative, relativeErr := filepath.Rel(repositoryRoot, position.Filename)
				if relativeErr != nil {
					relative = position.Filename
				}
				t.Errorf("%s:%d contains DELETE without a statically verified WHERE clause: %q",
					relative, position.Line, statement)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("scan Go sources for unbounded DELETE statements: %v", err)
	}
}

func TestUnboundedDeleteStatementDetection(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  int
	}{
		{name: "bounded", query: "DELETE FROM sessions WHERE username = ?", want: 0},
		{name: "unbounded", query: "DELETE FROM sessions", want: 1},
		{name: "multiline", query: "DELETE\nFROM sessions\nWHERE expired_at < ?", want: 0},
		{name: "mixed statements", query: "DELETE FROM sessions; DELETE FROM tokens WHERE name = ?", want: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := len(unboundedDeleteStatements(test.query)); got != test.want {
				t.Fatalf("detected %d unbounded DELETE statements, want %d", got, test.want)
			}
		})
	}
}
