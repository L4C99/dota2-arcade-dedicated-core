package core

// Linux's original early-exit fixture remains unchanged. The Windows helper
// synchronizes its platform-specific image-query/exit-signaling ambiguity.
func confirmEarlyExitBeforeObservation(m *Manager) func() error {
	return func() error { return nil }
}
