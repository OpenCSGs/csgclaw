package utils

// CloneAnyMap returns a shallow copy of src, or nil if src is nil or has no entries.
func CloneAnyMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]any, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

// CloneAnyMapShallowNestedStringMaps returns a shallow copy of src where each value typed as
// map[string]any is shallow-copied into a new map (so nested map[string]any is not shared with src).
// Other value kinds are assigned by reference like [CloneAnyMap]. Returns nil when len(src)==0.
func CloneAnyMapShallowNestedStringMaps(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]any, len(src))
	for k, v := range src {
		if sm, ok := v.(map[string]any); ok {
			c := make(map[string]any, len(sm))
			for ik, iv := range sm {
				c[ik] = iv
			}
			out[k] = c
		} else {
			out[k] = v
		}
	}
	return out
}

// OverlayAnyMap copies every entry from overlay onto dst (shallow). If overlay is empty, dst is returned unchanged.
// If dst is nil and overlay is non-empty, a new map is allocated. Otherwise dst is mutated in place.
func OverlayAnyMap(dst, overlay map[string]any) map[string]any {
	if len(overlay) == 0 {
		return dst
	}
	if dst == nil {
		dst = make(map[string]any, len(overlay))
	}
	for k, v := range overlay {
		dst[k] = v
	}
	return dst
}
