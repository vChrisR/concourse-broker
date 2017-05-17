package exec

import "os"

// TryStep wraps another step, ignores its errors, and always succeeds.
type TryStep struct {
	step    StepFactory
	runStep Step
}

// Try constructs a TryStep factory.
func Try(step StepFactory) TryStep {
	return TryStep{
		step: step,
	}
}

// Using constructs a *TryStep.
func (ts TryStep) Using(prev Step, repo *SourceRepository) Step {
	ts.runStep = ts.step.Using(prev, repo)
	return &ts
}

// Run runs the nested step, and always returns nil, ignoring the nested step's
// error.
func (ts *TryStep) Run(signals <-chan os.Signal, ready chan<- struct{}) error {
	err := ts.runStep.Run(signals, ready)
	if err == ErrInterrupted {
		return err
	}
	return nil
}

// Release releases the nested step.
func (ts *TryStep) Release() {
	ts.runStep.Release()
}

// Result indicates Success as true, and delegates everything else to the
// nested step.
func (ts *TryStep) Result(x interface{}) bool {
	switch v := x.(type) {
	case *Success:
		*v = Success(true)
		return true
	default:
		return ts.runStep.Result(x)
	}
}
