package state

// SeenWindow is an in-memory source-event window.
// Durable InboundLedger / DeliveryOutbox stay out of this package until a
// persistence owner exists.
type SeenWindow struct {
	limit int
	order []string
	items map[string]struct{}
}

func NewSeenWindow(limit int) *SeenWindow {
	if limit <= 0 {
		limit = 256
	}
	return &SeenWindow{limit: limit, items: make(map[string]struct{})}
}

func (s *SeenWindow) Has(key string) bool {
	if s == nil || key == "" {
		return false
	}
	_, ok := s.items[key]
	return ok
}

func (s *SeenWindow) Add(key string) {
	if s == nil || key == "" {
		return
	}
	if _, ok := s.items[key]; ok {
		return
	}
	s.items[key] = struct{}{}
	s.order = append(s.order, key)
	if len(s.order) <= s.limit {
		return
	}
	oldest := s.order[0]
	s.order = s.order[1:]
	delete(s.items, oldest)
}
