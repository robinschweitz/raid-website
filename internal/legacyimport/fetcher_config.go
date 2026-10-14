package legacyimport

// UseFetcher configures an authenticated fetch transport at startup before
// any import or sync call begins. It must not be called while the service runs.
func (s *Service) UseFetcher(fetcher Fetcher) {
    s.fetcher = fetcher
}
