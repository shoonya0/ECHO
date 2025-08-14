package utils

import (
	"errors"
	"fmt"
	"reflect"
)

// CopyMatchingFields copies values from src into dst **only** for fields/keys that exist in dst.
// dst must be a non-nil pointer to a settable value (struct, map, slice, etc).
func CopyMatchingFields(src, dst interface{}) error {
	if dst == nil {
		return errors.New("dst is nil")
	}
	// check if dst is a pointer
	dv := reflect.ValueOf(dst)
	if dv.Kind() != reflect.Ptr || dv.IsNil() {
		return errors.New("dst must be a non-nil pointer")
	}

	return copyValue(reflect.ValueOf(src), dv.Elem())
}

// copyValue does the recursive copying.
func copyValue(sv, dv reflect.Value) error {
	// Normalize interfaces / pointers on source:
	for sv.IsValid() && (sv.Kind() == reflect.Interface || sv.Kind() == reflect.Ptr) {
		if sv.IsNil() {
			// nothing to copy
			return nil
		}
		sv = sv.Elem()
	}
	// If destination is a pointer, ensure it is allocated and work on Elem()
	if dv.Kind() == reflect.Ptr {
		// if nil, allocate a new element
		if dv.IsNil() {
			dv.Set(reflect.New(dv.Type().Elem()))
		}
		return copyValue(sv, dv.Elem())
	}

	// If destination is an interface, attempt to set concrete value
	if dv.Kind() == reflect.Interface {
		if !sv.IsValid() {
			return nil
		}
		if sv.Type().AssignableTo(dv.Type()) {
			dv.Set(sv)
			return nil
		}
		// attempt conversion by creating a new value of the target interface type
		if sv.Type().ConvertibleTo(dv.Type()) {
			dv.Set(sv.Convert(dv.Type()))
			return nil
		}
		// fallback allow any value inside interface
		dv.Set(sv)
		return nil
	}

	switch dv.Kind() {
	case reflect.Struct:
		// iterate destination fields (we only copy into fields that exist on dst)
		dt := dv.Type()
		for i := 0; i < dv.NumField(); i++ {
			df := dt.Field(i)
			// only exported fields are settable
			dstField := dv.Field(i)
			if !dstField.CanSet() {
				continue
			}
			// Determine key name to look up in source map (if source is a map)
			key := jsonName(df)

			// Find matching value from source
			var srcFieldVal reflect.Value
			found := false
			if sv.IsValid() && sv.Kind() == reflect.Struct {
				sf := sv.FieldByName(df.Name)
				if sf.IsValid() {
					srcFieldVal = sf
					found = true
				}
			}
			if !found && sv.IsValid() && sv.Kind() == reflect.Map {
				// attempt to use json tag or field name as key in the map
				if key == "" {
					key = df.Name
				}
				mapKey := reflect.ValueOf(key)
				mv := sv.MapIndex(mapKey)
				if mv.IsValid() {
					srcFieldVal = mv
					found = true
				}
			}
			// if found in source, recursively copy
			if found && srcFieldVal.IsValid() {
				if err := copyValue(srcFieldVal, dstField); err != nil {
					return fmt.Errorf("field %s: %w", df.Name, err)
				}
			}
		}
		return nil

	case reflect.Map:
		// Ensure destination map is initialized
		if dv.IsNil() {
			dv.Set(reflect.MakeMap(dv.Type()))
		}
		// We accept source maps or structs
		if !sv.IsValid() {
			return nil
		}
		// Get element type for map values
		valType := dv.Type().Elem()

		switch sv.Kind() {
		case reflect.Map:
			for _, k := range sv.MapKeys() {
				// only support string keys on destination map (common case)
				if k.Kind() != reflect.String {
					continue
				}
				srcVal := sv.MapIndex(k)
				// prepare destination value
				newVal := reflect.New(valType).Elem()
				if err := copyValue(srcVal, newVal); err != nil {
					return fmt.Errorf("map key %v: %w", k.Interface(), err)
				}
				dv.SetMapIndex(k.Convert(dv.Type().Key()), newVal)
			}
		case reflect.Struct:
			// convert struct fields into map entries when dst is a map
			st := sv.Type()
			for i := 0; i < sv.NumField(); i++ {
				sf := st.Field(i)
				key := jsonName(sf)
				if key == "" {
					key = sf.Name
				}
				srcVal := sv.Field(i)
				mapKey := reflect.ValueOf(key)
				newVal := reflect.New(valType).Elem()
				if err := copyValue(srcVal, newVal); err != nil {
					return fmt.Errorf("struct->map key %s: %w", key, err)
				}
				dv.SetMapIndex(mapKey.Convert(dv.Type().Key()), newVal)
			}
		default:
			// try to convert scalar to map value? skip
		}
		return nil

	case reflect.Slice:
		if !sv.IsValid() {
			return nil
		}
		// handle if source is a slice/array
		if sv.Kind() == reflect.Slice || sv.Kind() == reflect.Array {
			n := sv.Len()
			newSlice := reflect.MakeSlice(dv.Type(), n, n)
			for i := 0; i < n; i++ {
				if err := copyValue(sv.Index(i), newSlice.Index(i)); err != nil {
					return fmt.Errorf("slice index %d: %w", i, err)
				}
			}
			dv.Set(newSlice)
			return nil
		}
		// if source is a single value and dest is slice, attempt to convert single element
		elemType := dv.Type().Elem()
		newSlice := reflect.MakeSlice(dv.Type(), 1, 1)
		newElem := reflect.New(elemType).Elem()
		if err := copyValue(sv, newElem); err != nil {
			return err
		}
		newSlice.Index(0).Set(newElem)
		dv.Set(newSlice)
		return nil

	default:
		// Basic types: try assign or convert
		if !sv.IsValid() {
			return nil
		}
		// If source is map/struct and dest is basic type, cannot copy
		if sv.Kind() == reflect.Map || sv.Kind() == reflect.Struct || sv.Kind() == reflect.Slice || sv.Kind() == reflect.Func {
			return fmt.Errorf("cannot assign %s to %s", sv.Kind(), dv.Kind())
		}
		// If assignable
		if sv.Type().AssignableTo(dv.Type()) {
			dv.Set(sv)
			return nil
		}
		// If convertible
		if sv.Type().ConvertibleTo(dv.Type()) {
			dv.Set(sv.Convert(dv.Type()))
			return nil
		}
		// Try simple cases such as numeric -> string? skip for safety
		return fmt.Errorf("incompatible types: %s -> %s", sv.Type(), dv.Type())
	}
}

// jsonName returns the json tag name (first token) if present, else empty string.
func jsonName(f reflect.StructField) string {
	tag := f.Tag.Get("json")
	if tag == "" {
		return ""
	}
	// take first token before comma, skip "-" tag
	for i, ch := range tag {
		if ch == ',' {
			tag = tag[:i]
			break
		}
	}
	if tag == "-" {
		return ""
	}
	return tag
}
