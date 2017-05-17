package exec

import (
	"os"
	"time"

	"code.cloudfoundry.org/clock"
	"github.com/tedsuo/ifrit"
)

// TimeoutStep applies a fixed timeout to a step's Run.
type TimeoutStep struct {
	step     StepFactory
	runStep  Step
	duration string
	clock    clock.Clock
	timedOut bool
}

// Timeout constructs a TimeoutStep factory.
func Timeout(
	step StepFactory,
	duration string,
	clock clock.Clock,
) TimeoutStep {
	return TimeoutStep{
		step:     step,
		duration: duration,
		clock:    clock,
	}
}

// Using constructs a *TimeoutStep.
func (ts TimeoutStep) Using(prev Step, repo *SourceRepository) Step {
	ts.runStep = ts.step.Using(prev, repo)

	return &ts
}

// Run parses the timeout duration and invokes the nested step.
//
// If the nested step takes longer than the duration, it is sent the Interrupt
// signal, and the TimeoutStep returns nil once the nested step exits (ignoring
// the nested step's error).
//
// The result of the nested step's Run is returned.
func (ts *TimeoutStep) Run(signals <-chan os.Signal, ready chan<- struct{}) error {
	parsedDuration, err := time.ParseDuration(ts.duration)
	if err != nil {
		return err
	}

	timer := ts.clock.NewTimer(parsedDuration)

	runProcess := ifrit.Invoke(ts.runStep)

	close(ready)

	var runErr error
	var sig os.Signal

dance:
	for {
		select {
		case runErr = <-runProcess.Wait():
			break dance
		case <-timer.C():
			ts.timedOut = true
			runProcess.Signal(os.Interrupt)
		case sig = <-signals:
			runProcess.Signal(sig)
		}
	}

	if ts.timedOut {
		// swallow interrupted error
		return nil
	}

	if runErr != nil {
		return runErr
	}

	return nil
}

// Release releases the nested step.
func (ts *TimeoutStep) Release() {
	ts.runStep.Release()
}

// Result indicates Success as true if the nested step completed successfully
// and did not time out.
//
// Any other type is ignored.
func (ts *TimeoutStep) Result(x interface{}) bool {
	switch v := x.(type) {
	case *Success:
		var success Success
		ts.runStep.Result(&success)
		*v = success && !Success(ts.timedOut)
		return true
	}
	return false
}
