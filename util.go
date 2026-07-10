// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-ruby-puppet-resource-api/puppet-resource-api authors

package resourceapi

import (
	"reflect"
	"sort"
)

// sortedKeys returns the keys of a change map in sorted order for deterministic
// iteration.
func sortedKeys(m map[string]Change) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// sortedResourceKeys returns the keys of a resource map in sorted order.
func sortedResourceKeys(m map[string]Resource) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// equalAny reports value equality of two attribute values, widening integer and
// float kinds so that, e.g., an int64 from a provider compares equal to an int
// from a manifest.
func equalAny(a, b any) bool {
	return reflect.DeepEqual(normalize(a), normalize(b))
}

// normalize widens numeric kinds and recurses into slices and string-keyed maps
// so that equalAny is insensitive to the concrete Go integer/float type used. A
// [*Sensitive] is unwrapped first so a sensitive value still compares by its
// content.
func normalize(v any) any {
	switch x := v.(type) {
	case *Sensitive:
		return normalize(x.value)
	case int:
		return int64(x)
	case int8:
		return int64(x)
	case int16:
		return int64(x)
	case int32:
		return int64(x)
	case int64:
		return x
	case uint:
		return int64(x)
	case uint8:
		return int64(x)
	case uint16:
		return int64(x)
	case uint32:
		return int64(x)
	case uint64:
		return int64(x)
	case float32:
		return float64(x)
	case float64:
		return x
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = normalize(e)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[k] = normalize(e)
		}
		return out
	default:
		return v
	}
}
