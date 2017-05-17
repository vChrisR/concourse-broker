package worker_test

import (
	"errors"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/ginkgomon"

	"code.cloudfoundry.org/clock/fakeclock"
	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/lager/lagertest"

	"github.com/concourse/atc"
	"github.com/concourse/atc/db"
	"github.com/concourse/atc/worker"
	"github.com/concourse/atc/worker/workerfakes"
)

var _ = Describe("Hardcoded", func() {
	var (
		logger           lager.Logger
		workerDB         *workerfakes.FakeSaveWorkerDB
		gardenAddr       string
		baggageClaimAddr string
		resourceTypes    []atc.WorkerResourceType
		fakeClock        *fakeclock.FakeClock

		process ifrit.Process
	)

	BeforeEach(func() {
		logger = lagertest.NewTestLogger("hardcoded-worker")
		workerDB = &workerfakes.FakeSaveWorkerDB{}
		gardenAddr = "http://garden.example.com"
		baggageClaimAddr = "http://volumes.example.com"
		resourceTypes = []atc.WorkerResourceType{
			{
				Type:  "type",
				Image: "image",
			},
		}
		fakeClock = fakeclock.NewFakeClock(time.Now())
	})

	Describe("registering a single worker", func() {
		JustBeforeEach(func() {
			runner := worker.NewHardcoded(logger, workerDB, fakeClock, gardenAddr, baggageClaimAddr, resourceTypes)
			process = ginkgomon.Invoke(runner)
		})

		AfterEach(func() {
			ginkgomon.Interrupt(process)
		})

		It("registers it and then keeps registering it on an interval", func() {
			expectedWorkerInfo := db.WorkerInfo{
				Name:             gardenAddr,
				GardenAddr:       gardenAddr,
				BaggageclaimURL:  baggageClaimAddr,
				ActiveContainers: 0,
				ResourceTypes:    resourceTypes,
				Platform:         "linux",
				Tags:             []string{},
			}
			expectedTTL := 30 * time.Second

			Eventually(workerDB.SaveWorkerCallCount()).Should(Equal(1))
			workerInfo, ttl := workerDB.SaveWorkerArgsForCall(0)
			Expect(workerInfo).To(Equal(expectedWorkerInfo))
			Expect(ttl).To(Equal(expectedTTL))

			fakeClock.Increment(11 * time.Second)

			Eventually(workerDB.SaveWorkerCallCount).Should(Equal(2))
			workerInfo, ttl = workerDB.SaveWorkerArgsForCall(1)
			Expect(workerInfo).To(Equal(expectedWorkerInfo))
			Expect(ttl).To(Equal(expectedTTL))
		})

		It("can be interrupted", func() {
			expectedWorkerInfo := db.WorkerInfo{
				Name:             gardenAddr,
				GardenAddr:       gardenAddr,
				BaggageclaimURL:  baggageClaimAddr,
				ActiveContainers: 0,
				ResourceTypes:    resourceTypes,
				Platform:         "linux",
				Tags:             []string{},
			}
			expectedTTL := 30 * time.Second

			Eventually(workerDB.SaveWorkerCallCount()).Should(Equal(1))
			workerInfo, ttl := workerDB.SaveWorkerArgsForCall(0)
			Expect(workerInfo).To(Equal(expectedWorkerInfo))
			Expect(ttl).To(Equal(expectedTTL))

			ginkgomon.Interrupt(process)

			fakeClock.Increment(11 * time.Second)

			Consistently(workerDB.SaveWorkerCallCount).Should(Equal(1))
		})
	})

	Context("if saving to the DB fails", func() {
		disaster := errors.New("bad bad bad")

		BeforeEach(func() {
			workerDB.SaveWorkerReturns(db.SavedWorker{}, disaster)
		})

		It("exits early", func() {
			runner := worker.NewHardcoded(logger, workerDB, fakeClock, gardenAddr, baggageClaimAddr, resourceTypes)
			process = ifrit.Invoke(runner)

			Expect(<-process.Wait()).To(Equal(disaster))
		})
	})
})
