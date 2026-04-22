package config

// rebuildIndexes must be called with the lock already held (or during init).
func (s *Store) rebuildIndexes() {
	prevStatus := s.accTest
	s.keyMap = make(map[string]struct{}, len(s.cfg.Keys))
	for _, k := range s.cfg.Keys {
		s.keyMap[k] = struct{}{}
	}
	s.accMap = make(map[string]int, len(s.accounts))
	s.accTest = make(map[string]string, len(s.accounts))
	for i, acc := range s.accounts {
		id := acc.Identifier()
		if id != "" {
			s.accMap[id] = i
			if status, ok := prevStatus[id]; ok {
				s.setAccountTestStatusLocked(acc, status, "")
			}
		}
	}
}

// findAccountIndexLocked expects the store lock to already be held.
func (s *Store) findAccountIndexLocked(identifier string) (int, bool) {
	if idx, ok := s.accMap[identifier]; ok && idx >= 0 && idx < len(s.accounts) {
		return idx, true
	}
	// Fallback for token-only accounts whose derived identifier changed after
	// a token refresh; this preserves correctness on map misses.
	for i, acc := range s.accounts {
		if acc.Identifier() == identifier {
			return i, true
		}
	}
	return -1, false
}

func (s *Store) setAccountTestStatusLocked(acc Account, status, hintedIdentifier string) {
	status = lower(status)
	if status == "" {
		return
	}
	if id := acc.Identifier(); id != "" {
		s.accTest[id] = status
	}
	if email := acc.Email; email != "" {
		s.accTest[email] = status
	}
	if mobile := CanonicalMobileKey(acc.Mobile); mobile != "" {
		s.accTest[mobile] = status
	}
	if hintedIdentifier = lower(hintedIdentifier); hintedIdentifier != "" {
		s.accTest[hintedIdentifier] = status
	}
}
