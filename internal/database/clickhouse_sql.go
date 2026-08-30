/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

package database

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
)

var clickHouseMutationPattern = regexp.MustCompile(`(?is)^\s*(?:/\*.*?\*/\s*)*(INSERT\s+INTO|ALTER\s+TABLE|DELETE\s+FROM|TRUNCATE\s+TABLE)\s+([[:alnum:]_]+)`)

func clickHouseInsertKeys(table, query string, args []any) ([]string, error) {
	schema, found := clickHouseSchema(table)
	if !found {
		return nil, fmt.Errorf("ClickHouse schema for transaction table %s is missing", table)
	}
	match := clickHouseMutationPattern.FindStringIndex(query)
	if match == nil {
		return nil, fmt.Errorf("parse ClickHouse INSERT target %s", table)
	}
	remainder := query[match[1]:]
	columnStart := strings.Index(remainder, "(")
	if columnStart < 0 {
		return nil, fmt.Errorf("ClickHouse INSERT into %s must name its columns", table)
	}
	columnEnd := matchingSQLParenthesis(remainder, columnStart)
	if columnEnd < 0 {
		return nil, fmt.Errorf("ClickHouse INSERT column list for %s is invalid", table)
	}
	columns := splitTopLevelSQLList(remainder[columnStart+1 : columnEnd])
	valueSource := strings.TrimSpace(remainder[columnEnd+1:])
	upperSource := strings.ToUpper(valueSource)
	var expressions []string
	switch {
	case strings.HasPrefix(upperSource, "VALUES"):
		valueStart := strings.Index(valueSource, "(")
		if valueStart < 0 {
			return nil, fmt.Errorf("ClickHouse INSERT values for %s are invalid", table)
		}
		valueEnd := matchingSQLParenthesis(valueSource, valueStart)
		if valueEnd < 0 {
			return nil, fmt.Errorf("ClickHouse INSERT values for %s are invalid", table)
		}
		expressions = splitTopLevelSQLList(valueSource[valueStart+1 : valueEnd])
	case strings.HasPrefix(upperSource, "SELECT"):
		selectValues := strings.TrimSpace(valueSource[len("SELECT"):])
		if where := findTopLevelSQLKeyword(selectValues, "WHERE"); where >= 0 {
			selectValues = strings.TrimSpace(selectValues[:where])
		}
		expressions = splitTopLevelSQLList(selectValues)
	default:
		return nil, fmt.Errorf("ClickHouse INSERT into %s uses an unsupported source", table)
	}
	if len(columns) != len(expressions) {
		return nil, fmt.Errorf("ClickHouse INSERT into %s has %d columns and %d values", table, len(columns), len(expressions))
	}
	values := make(map[string]any, len(schema.keyColumns))
	argumentIndex := 0
	for index, expression := range expressions {
		placeholderCount := countSQLPlaceholders(expression)
		column := strings.Trim(strings.TrimSpace(columns[index]), "`\"")
		for _, keyColumn := range schema.keyColumns {
			if column != keyColumn {
				continue
			}
			if strings.TrimSpace(expression) == "?" && argumentIndex < len(args) {
				values[keyColumn] = args[argumentIndex]
			} else if placeholderCount == 0 {
				values[keyColumn] = parseSimpleSQLLiteral(expression)
			} else {
				return nil, fmt.Errorf("ClickHouse transaction key %s.%s uses an unsupported expression", table, keyColumn)
			}
		}
		argumentIndex += placeholderCount
	}
	keyValues := make([]any, 0, len(schema.keyColumns))
	for _, keyColumn := range schema.keyColumns {
		value, exists := values[keyColumn]
		if !exists {
			return nil, fmt.Errorf("ClickHouse INSERT into %s omits transaction key column %s", table, keyColumn)
		}
		keyValues = append(keyValues, value)
	}
	return []string{encodeClickHouseKey(keyValues)}, nil
}

func clickHouseSchema(table string) (clickHouseTableSchema, bool) {
	for _, schema := range clickHouseSchemas() {
		if schema.name == table {
			return schema, true
		}
	}
	return clickHouseTableSchema{}, false
}

func encodeClickHouseKey(values []any) string {
	var result strings.Builder
	for _, value := range values {
		if value == nil {
			result.WriteByte('N')
			continue
		}
		text := fmt.Sprint(value)
		result.WriteString(fmt.Sprintf("S%d:%s", len(text), text))
	}
	return result.String()
}

func matchingSQLParenthesis(query string, start int) int {
	depth := 0
	quote := byte(0)
	for index := start; index < len(query); index++ {
		character := query[index]
		if quote != 0 {
			if character == quote {
				if index+1 < len(query) && query[index+1] == quote {
					index++
					continue
				}
				quote = 0
			}
			continue
		}
		switch character {
		case '\'', '"', '`':
			quote = character
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return index
			}
		}
	}
	return -1
}

func splitTopLevelSQLList(value string) []string {
	parts := make([]string, 0)
	start := 0
	depth := 0
	quote := byte(0)
	for index := 0; index < len(value); index++ {
		character := value[index]
		if quote != 0 {
			if character == quote {
				if index+1 < len(value) && value[index+1] == quote {
					index++
					continue
				}
				quote = 0
			}
			continue
		}
		switch character {
		case '\'', '"', '`':
			quote = character
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				parts = append(parts, strings.TrimSpace(value[start:index]))
				start = index + 1
			}
		}
	}
	parts = append(parts, strings.TrimSpace(value[start:]))
	return parts
}

func parseSimpleSQLLiteral(value string) any {
	trimmed := strings.TrimSpace(value)
	if strings.EqualFold(trimmed, "NULL") {
		return nil
	}
	if len(trimmed) >= 2 && trimmed[0] == '\'' && trimmed[len(trimmed)-1] == '\'' {
		return strings.ReplaceAll(trimmed[1:len(trimmed)-1], "''", "'")
	}
	return trimmed
}

func clickHouseMutationTable(query string) string {
	match := clickHouseMutationPattern.FindStringSubmatch(query)
	if len(match) != 3 || !validClickHouseIdentifier(match[2]) {
		return ""
	}
	return match[2]
}

func validClickHouseIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character != '_' && (character < 'a' || character > 'z') && (character < 'A' || character > 'Z') &&
			(character < '0' || character > '9') {
			return false
		}
	}
	return true
}

func stripLeadingSQLComments(query string) string {
	trimmed := strings.TrimSpace(query)
	for strings.HasPrefix(trimmed, "/*") {
		end := strings.Index(trimmed, "*/")
		if end < 0 {
			break
		}
		trimmed = strings.TrimSpace(trimmed[end+2:])
	}
	return trimmed
}

func findTopLevelSQLKeyword(query, keyword string) int {
	upper := strings.ToUpper(query)
	depth := 0
	quote := byte(0)
	for index := 0; index < len(query); index++ {
		character := query[index]
		if quote != 0 {
			if character == quote {
				if index+1 < len(query) && query[index+1] == quote {
					index++
					continue
				}
				quote = 0
			} else if character == '\\' && index+1 < len(query) {
				index++
			}
			continue
		}
		switch character {
		case '\'', '"', '`':
			quote = character
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		default:
			if depth == 0 && strings.HasPrefix(upper[index:], keyword) &&
				(index == 0 || !isSQLIdentifierByte(upper[index-1])) &&
				(index+len(keyword) == len(upper) || !isSQLIdentifierByte(upper[index+len(keyword)])) {
				return index
			}
		}
	}
	return -1
}

func countSQLPlaceholders(query string) int {
	count := 0
	quote := byte(0)
	for index := 0; index < len(query); index++ {
		character := query[index]
		if quote != 0 {
			if character == quote {
				if index+1 < len(query) && query[index+1] == quote {
					index++
					continue
				}
				quote = 0
			} else if character == '\\' && index+1 < len(query) {
				index++
			}
			continue
		}
		if character == '\'' || character == '"' || character == '`' {
			quote = character
		} else if character == '?' {
			count++
		}
	}
	return count
}

func isSQLIdentifierByte(value byte) bool {
	return value == '_' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9'
}

func normalizeClickHouseArguments(arguments []any) []any {
	result := arguments
	for index, argument := range arguments {
		value := reflect.ValueOf(argument)
		if !value.IsValid() || value.Kind() != reflect.Slice || value.Type().Elem().Kind() != reflect.Uint8 {
			continue
		}
		if &result[0] == &arguments[0] {
			result = append([]any(nil), arguments...)
		}
		result[index] = string(value.Bytes())
	}
	return result
}
