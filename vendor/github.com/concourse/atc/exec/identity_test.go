package exec_test

import (
	"os"

	. "github.com/concourse/atc/exec"

	"github.com/concourse/atc/exec/execfakes"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Identity", func() {
	var (
		inStep *execfakes.FakeStep
		repo   *SourceRepository

		identity Identity

		step Step
	)

	BeforeEach(func() {
		identity = Identity{}

		inStep = new(execfakes.FakeStep)
		repo = NewSourceRepository()
	})

	JustBeforeEach(func() {
		step = identity.Using(inStep, repo)
	})

	Describe("Run", func() {
		It("is a no-op", func() {
			ready := make(chan struct{})
			signals := make(chan os.Signal)

			err := step.Run(signals, ready)
			Expect(err).NotTo(HaveOccurred())

			Expect(inStep.RunCallCount()).To(BeZero())
		})
	})

	Describe("Result", func() {
		It("calls through to the input source", func() {
			var result int
			step.Result(&result)

			Expect(inStep.ResultCallCount()).To(Equal(1))
			Expect(inStep.ResultArgsForCall(0)).To(Equal(&result))
		})
	})
})
