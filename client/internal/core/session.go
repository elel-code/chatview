package core

func (s *Service) PublicKey() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.publicKey
}

func (s *Service) IsOffline() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.offline
}

func (s *Service) setSession(publicKey string, role int32, offline bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.publicKey = publicKey
	s.role = role
	s.offline = offline
}

func (s *Service) setCacheOwner(publicKey string) error {
	if s.cache == nil {
		return nil
	}
	return s.cache.SetOwner(publicKey)
}
