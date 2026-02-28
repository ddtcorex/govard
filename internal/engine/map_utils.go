package engine

// MergeMap recursively merges the src map into the dst map.
// It skips nil values in the source map.
func MergeMap(dst, src map[string]any) {
	for key, val := range src {
		if val == nil {
			if _, exists := dst[key]; !exists {
				dst[key] = nil
			}
			continue
		}

		srcMap, srcIsMap := val.(map[string]any)
		dstMap, dstIsMap := dst[key].(map[string]any)

		if srcIsMap && dstIsMap {
			MergeMap(dstMap, srcMap)
			dst[key] = dstMap
			continue
		}

		dst[key] = val
	}
}
