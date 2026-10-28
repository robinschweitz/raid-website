package auth

import "errors"

// playerError carries the player of a failed MFA step to the caller for the
// security journal (SC-041). It unwraps to the original error, so errors.Is
// keeps working.
type playerError struct {
	playerID int64
	err      error
}

func (e *playerError) Error() string { return e.err.Error() }
func (e *playerError) Unwrap() error { return e.err }

func withPlayer(playerID int64, err error) error {
	if err == nil {
		return nil
	}
	return &playerError{playerID: playerID, err: err}
}

// PlayerIDFromError returns the player that a failed MFA verification
// belonged to. An unknown or expired challenge has none.
func PlayerIDFromError(err error) (int64, bool) {
	var pe *playerError
	if errors.As(err, &pe) {
		return pe.playerID, true
	}
	return 0, false
}
