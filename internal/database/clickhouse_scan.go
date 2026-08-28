/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0.
 */

package database

import (
	"database/sql"
	"errors"
	"fmt"
	"reflect"

	chdriver "github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

func scanClickHouseValues(source chdriver.Rows, destinations ...any) error {
	columnTypes := source.ColumnTypes()
	if len(columnTypes) != len(destinations) {
		return fmt.Errorf("ClickHouse scan destination count %d does not match column count %d", len(destinations), len(columnTypes))
	}
	targets := make([]any, len(columnTypes))
	for index, columnType := range columnTypes {
		targets[index] = reflect.New(columnType.ScanType()).Interface()
	}
	if err := source.Scan(targets...); err != nil {
		return err
	}
	for index, target := range targets {
		value := reflect.ValueOf(target).Elem().Interface()
		if err := assignClickHouseValue(destinations[index], value); err != nil {
			return fmt.Errorf("convert ClickHouse column %s: %w", columnTypes[index].Name(), err)
		}
	}
	return nil
}

func assignClickHouseValue(destination, source any) error {
	destinationValue := reflect.ValueOf(destination)
	if !destinationValue.IsValid() || destinationValue.Kind() != reflect.Pointer || destinationValue.IsNil() {
		return errors.New("scan destination is not a writable pointer")
	}
	sourceValue := reflect.ValueOf(source)
	for sourceValue.IsValid() && sourceValue.Kind() == reflect.Pointer {
		if sourceValue.IsNil() {
			sourceValue = reflect.Value{}
			break
		}
		sourceValue = sourceValue.Elem()
	}
	var raw any
	if sourceValue.IsValid() {
		raw = sourceValue.Interface()
	}
	if scanner, ok := destination.(sql.Scanner); ok {
		return scanner.Scan(raw)
	}
	target := destinationValue.Elem()
	if !sourceValue.IsValid() {
		target.SetZero()
		return nil
	}
	if sourceValue.Type().AssignableTo(target.Type()) {
		target.Set(sourceValue)
		return nil
	}
	if target.Kind() == reflect.Slice && target.Type().Elem().Kind() == reflect.Uint8 {
		switch typed := raw.(type) {
		case string:
			target.SetBytes([]byte(typed))
			return nil
		case []byte:
			target.SetBytes(typed)
			return nil
		}
	}
	if target.Kind() == reflect.String {
		switch typed := raw.(type) {
		case string:
			target.SetString(typed)
			return nil
		case []byte:
			target.SetString(string(typed))
			return nil
		}
	}
	if isReflectInteger(target.Kind()) && isReflectInteger(sourceValue.Kind()) {
		if target.Kind() >= reflect.Int && target.Kind() <= reflect.Int64 {
			value, ok := reflectIntegerAsInt64(sourceValue)
			if !ok || target.OverflowInt(value) {
				return fmt.Errorf("integer %v overflows %s", raw, target.Type())
			}
			target.SetInt(value)
			return nil
		}
		value, ok := reflectIntegerAsUint64(sourceValue)
		if !ok || target.OverflowUint(value) {
			return fmt.Errorf("integer %v overflows %s", raw, target.Type())
		}
		target.SetUint(value)
		return nil
	}
	if target.Kind() == reflect.Bool && isReflectInteger(sourceValue.Kind()) {
		value, ok := reflectIntegerAsUint64(sourceValue)
		if !ok {
			return fmt.Errorf("integer %v cannot convert to bool", raw)
		}
		target.SetBool(value != 0)
		return nil
	}
	if sourceValue.Type().ConvertibleTo(target.Type()) {
		target.Set(sourceValue.Convert(target.Type()))
		return nil
	}
	if target.Kind() == reflect.Interface {
		target.Set(sourceValue)
		return nil
	}
	return fmt.Errorf("unsupported conversion from %T to %T", raw, destination)
}

func isReflectInteger(kind reflect.Kind) bool {
	return kind >= reflect.Int && kind <= reflect.Int64 || kind >= reflect.Uint && kind <= reflect.Uint64
}

func reflectIntegerAsInt64(value reflect.Value) (int64, bool) {
	if value.Kind() >= reflect.Int && value.Kind() <= reflect.Int64 {
		return value.Int(), true
	}
	unsigned := value.Uint()
	if unsigned > uint64(^uint64(0)>>1) {
		return 0, false
	}
	return int64(unsigned), true
}

func reflectIntegerAsUint64(value reflect.Value) (uint64, bool) {
	if value.Kind() >= reflect.Uint && value.Kind() <= reflect.Uint64 {
		return value.Uint(), true
	}
	signed := value.Int()
	if signed < 0 {
		return 0, false
	}
	return uint64(signed), true
}
