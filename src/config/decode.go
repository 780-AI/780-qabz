package config

import (
	"fmt"
	"reflect"
	"strings"
	"time"
)

// The mapper assigns a YAML node to a Go value. The node kind must fit the Go type:
//
//	YAML array  -> a slice or an array ([]T)
//	YAML object -> a struct, or a map[string]T
//	YAML scalar -> int*, uint*, bool, string, time.Duration (a duration string such as "30s")
//
// Any other pairing is an error, at any depth. Errors name the path and the kinds, never a value,
// because a value can be a secret. The mapper runs at bootstrap, never on a hot path.

var durationType = reflect.TypeFor[time.Duration]()

// decodeStruct fills the tagged fields of rv from m. The tag `path` is a dotted path relative to m.
// A field with no tag is skipped. A path that is missing leaves the field as it is.
func decodeStruct(m map[string]any, rv reflect.Value, prefix string) error {
	if !rv.IsValid() || rv.Kind() != reflect.Struct || !rv.CanSet() {
		return fmt.Errorf("config %s: the target must be a settable struct", strings.TrimSuffix(prefix, "."))
	}
	t := rv.Type()
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		path, ok := f.Tag.Lookup("path")
		if !ok || !f.IsExported() {
			continue
		}
		node, found, err := lookup(m, path, prefix)
		if err != nil {
			return err
		}
		if !found {
			continue
		}
		if err := decodeValue(node, rv.Field(i), prefix+path); err != nil {
			return err
		}
	}
	return nil
}

// lookup walks a dotted path in m. found is false when a key is missing.
func lookup(m map[string]any, path, prefix string) (node any, found bool, err error) {
	cur := any(m)
	parts := strings.Split(path, ".")
	for i, p := range parts {
		obj, ok := cur.(map[string]any)
		if !ok {
			return nil, false, fmt.Errorf("config %s%s: %s is %s, want an object", prefix, path, strings.Join(parts[:i], "."), kindOf(cur))
		}
		cur, ok = obj[p]
		if !ok {
			return nil, false, nil
		}
	}
	return cur, true, nil
}

func decodeValue(node any, rv reflect.Value, path string) error {
	if node == nil {
		return fmt.Errorf("config %s: null is not allowed", path)
	}
	if rv.Type() == durationType {
		s, ok := node.(string)
		if !ok {
			return mismatch(path, node, "a duration string")
		}
		d, err := time.ParseDuration(s)
		if err != nil {
			return fmt.Errorf("config %s: not a valid duration (for example 30s)", path)
		}
		rv.SetInt(int64(d))
		return nil
	}
	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		arr, ok := node.([]any)
		if !ok {
			return mismatch(path, node, "an array")
		}
		if rv.Kind() == reflect.Array {
			if len(arr) != rv.Len() {
				return fmt.Errorf("config %s: array has %d elements, want %d", path, len(arr), rv.Len())
			}
		} else {
			rv.Set(reflect.MakeSlice(rv.Type(), len(arr), len(arr)))
		}
		for i, el := range arr {
			if err := decodeValue(el, rv.Index(i), fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	case reflect.Struct:
		obj, ok := node.(map[string]any)
		if !ok {
			return mismatch(path, node, "an object")
		}
		return decodeStruct(obj, rv, path+".")
	case reflect.Map:
		if rv.Type().Key().Kind() != reflect.String {
			return fmt.Errorf("config %s: map key type %s is not supported, use string", path, rv.Type().Key())
		}
		obj, ok := node.(map[string]any)
		if !ok {
			return mismatch(path, node, "an object")
		}
		out := reflect.MakeMapWithSize(rv.Type(), len(obj))
		for k, v := range obj {
			el := reflect.New(rv.Type().Elem()).Elem()
			if err := decodeValue(v, el, path+"."+k); err != nil {
				return err
			}
			out.SetMapIndex(reflect.ValueOf(k).Convert(rv.Type().Key()), el)
		}
		rv.Set(out)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, ok := asInt(node)
		if !ok {
			return mismatch(path, node, "an integer")
		}
		if rv.OverflowInt(n) {
			return fmt.Errorf("config %s: integer out of range for %s", path, rv.Type())
		}
		rv.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, ok := asUint(node)
		if !ok {
			return mismatch(path, node, "a non-negative integer")
		}
		if rv.OverflowUint(n) {
			return fmt.Errorf("config %s: integer out of range for %s", path, rv.Type())
		}
		rv.SetUint(n)
	case reflect.String:
		s, ok := node.(string)
		if !ok {
			return mismatch(path, node, "a string")
		}
		rv.SetString(s)
	case reflect.Bool:
		b, ok := node.(bool)
		if !ok {
			return mismatch(path, node, "a boolean")
		}
		rv.SetBool(b)
	default:
		return fmt.Errorf("config %s: Go type %s is not supported", path, rv.Type())
	}
	return nil
}

func asInt(node any) (int64, bool) {
	switch n := node.(type) {
	case int:
		return int64(n), true
	case int64:
		return n, true
	case uint64:
		if n > 1<<63-1 {
			return 0, false
		}
		return int64(n), true
	}
	return 0, false
}

func asUint(node any) (uint64, bool) {
	switch n := node.(type) {
	case int:
		return uint64(n), n >= 0
	case int64:
		return uint64(n), n >= 0
	case uint64:
		return n, true
	}
	return 0, false
}

func mismatch(path string, node any, want string) error {
	return fmt.Errorf("config %s: is %s, want %s", path, kindOf(node), want)
}

func kindOf(node any) string {
	switch node.(type) {
	case []any:
		return "an array"
	case map[string]any:
		return "an object"
	case string:
		return "a string"
	case bool:
		return "a boolean"
	case int, int64, uint64:
		return "an integer"
	case float64:
		return "a float"
	case nil:
		return "null"
	}
	return fmt.Sprintf("a %T", node)
}

func reflectValue(p any) reflect.Value { return reflect.ValueOf(p).Elem() }
