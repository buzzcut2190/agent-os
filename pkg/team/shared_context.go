package team

import (
	"time"
)

// PutContext stores or updates a SharedContext entry. If ctx.ID is empty a
// UUID is generated. If an entry with the same ID already exists it is
// replaced.
func (s *TeamStore) PutContext(ctx SharedContext) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if ctx.ID == "" {
		ctx.ID = newUUID()
	}
	if ctx.Created.IsZero() {
		ctx.Created = time.Now()
	}

	for i, existing := range s.contexts {
		if existing.ID == ctx.ID {
			s.contexts[i] = &ctx
			return nil
		}
	}

	s.contexts = append(s.contexts, &ctx)
	return nil
}

// GetContext returns a copy of the SharedContext identified by id.
func (s *TeamStore) GetContext(id string) (*SharedContext, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, ctx := range s.contexts {
		if ctx.ID == id {
			cp := deepCopyContext(ctx)
			return &cp, true
		}
	}
	return nil, false
}

// ListContexts returns copies of all SharedContext entries that carry the
// given tag. If tag is empty all contexts are returned.
func (s *TeamStore) ListContexts(tag string) []*SharedContext {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*SharedContext
	for _, ctx := range s.contexts {
		if tag == "" || hasTag(ctx.Tags, tag) {
			cp := deepCopyContext(ctx)
			result = append(result, &cp)
		}
	}
	return result
}

// PurgeExpired removes all SharedContext entries whose Expiry time has
// passed. Entries with a zero Expiry never expire.
func (s *TeamStore) PurgeExpired() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	kept := s.contexts[:0]
	for _, ctx := range s.contexts {
		if ctx.Expiry.IsZero() || ctx.Expiry.After(now) {
			kept = append(kept, ctx)
		}
	}
	s.contexts = kept
}

func hasTag(tags []string, tag string) bool {
	for _, t := range tags {
		if t == tag {
			return true
		}
	}
	return false
}

func deepCopyContext(ctx *SharedContext) SharedContext {
	cp := *ctx
	cp.Authors = copySlice(ctx.Authors)
	cp.Tags = copySlice(ctx.Tags)
	if ctx.Metadata != nil {
		cp.Metadata = make(map[string]string, len(ctx.Metadata))
		for k, v := range ctx.Metadata {
			cp.Metadata[k] = v
		}
	}
	return cp
}
