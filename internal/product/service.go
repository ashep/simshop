package product

import "sort"

type Service struct {
	items []*Item
}

func NewService(items []*Item) *Service {
	if items == nil {
		items = []*Item{}
	}
	return &Service{items: items}
}

// Title returns the product's display title for use in places that have no
// per-request language context (e.g. the Telegram notifier). Picks the "en"
// entry when present, otherwise the alphabetically-first available language,
// so the result is deterministic. Returns "" when the product id is not in
// the catalog or has no title.
func (s *Service) Title(id string) string {
	for _, p := range s.items {
		if p.ID != id {
			continue
		}
		if v, ok := p.Title["en"]; ok && v != "" {
			return v
		}
		keys := make([]string, 0, len(p.Title))
		for k := range p.Title {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if v := p.Title[k]; v != "" {
				return v
			}
		}
		return ""
	}
	return ""
}
