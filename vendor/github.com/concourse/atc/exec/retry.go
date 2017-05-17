package exec

import "os"

// Retry constructs a Step that will run the steps in order until one of them
// succeeds.
type Retry []StepFactory

// Using constructs a *RetryStep.
func (stepFactory Retry) Using(prev Step, repo *SourceRepository) Step {
	retry := &RetryStep{}

	for _, subStepFactory := range stepFactory {
		retry.Attempts = append(retry.Attempts, subStepFactory.Using(prev, repo))
	}

	return retry
}

// RetryStep is a step that will run the steps in order until one of them
// succeeds.
type RetryStep struct {
	Attempts    []Step
	LastAttempt Step
}

// Run iterates through each step, stopping once a step succeeds. If all steps
// fail, the RetryStep will fail.
func (step *RetryStep) Run(signals <-chan os.Signal, ready chan<- struct{}) error {
	close(ready)

	var attemptErr error

	for _, attempt := range step.Attempts {
		step.LastAttempt = attempt

		var succeeded Success
		attemptErr = attempt.Run(signals, make(chan struct{}))
		if attemptErr == ErrInterrupted {
			return attemptErr
		}

		if attemptErr != nil {
			continue
		}

		if attempt.Result(&succeeded) && bool(succeeded) {
			break
		}
	}

	return attemptErr
}

// Release releases each nested step.
func (step *RetryStep) Release() {
	for _, src := range step.Attempts {
		src.Release()
	}
}

// Result delegates to the last step that it ran.
func (step *RetryStep) Result(x interface{}) bool {
	return step.LastAttempt.Result(x)
}
