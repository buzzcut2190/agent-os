package skill

import "fmt"

// AutoActivate restores the saved activation state on startup. Returns the list
// of skill names that were successfully reactivated.
func (e *Engine) AutoActivate() ([]string, error) {
	if err := e.LoadState(); err != nil {
		return nil, fmt.Errorf("auto-activate: load state: %w", err)
	}
	names := e.ActiveNames()
	return names, nil
}
