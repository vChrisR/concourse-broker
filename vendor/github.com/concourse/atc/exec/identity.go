package exec

import "os"

// Identity constructs a step that just propagates the previous step to the
// next one, without running anything.
type Identity struct{}

// Using constructs an IdentityStep.
func (Identity) Using(prev Step, repo *SourceRepository) Step {
	return IdentityStep{prev}
}

// IdentityStep does nothing, and delegates everything else to its nested step.
type IdentityStep struct {
	Step
}

// Run does nothing.
func (IdentityStep) Run(<-chan os.Signal, chan<- struct{}) error {
	return nil
}
