package modelcap

import "strings"

// VideoGeneration describes provider-advertised request constraints.
type VideoGeneration struct {
	Sizes   []string `json:"size,omitempty"`
	Seconds []int    `json:"seconds,omitempty"`
}

func (v VideoGeneration) Normalized() VideoGeneration {
	out := VideoGeneration{}
	seenSizes := map[string]struct{}{}
	for _, size := range v.Sizes {
		size = strings.TrimSpace(size)
		key := strings.ToLower(size)
		if size == "" {
			continue
		}
		if _, ok := seenSizes[key]; ok {
			continue
		}
		seenSizes[key] = struct{}{}
		out.Sizes = append(out.Sizes, size)
	}
	seenSeconds := map[int]struct{}{}
	for _, seconds := range v.Seconds {
		if seconds <= 0 {
			continue
		}
		if _, ok := seenSeconds[seconds]; ok {
			continue
		}
		seenSeconds[seconds] = struct{}{}
		out.Seconds = append(out.Seconds, seconds)
	}
	return out
}

func CloneVideoGeneration(in map[string]VideoGeneration) map[string]VideoGeneration {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]VideoGeneration, len(in))
	for model, capabilities := range in {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		out[model] = capabilities.Normalized()
	}
	return out
}
