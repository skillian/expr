package internal

// Sentinel is a placeholder error value.
type Sentinel struct{}

func (s *Sentinel) Error() string { return "sentinel" }
