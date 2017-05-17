package worker_test

import (
	"errors"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"code.cloudfoundry.org/clock/fakeclock"
	"code.cloudfoundry.org/lager/lagertest"
	"github.com/concourse/atc/worker"
	"github.com/concourse/atc/worker/workerfakes"
	"github.com/concourse/baggageclaim"
	bfakes "github.com/concourse/baggageclaim/baggageclaimfakes"
)

var _ = Describe("Volumes", func() {
	var (
		volumeFactory worker.VolumeFactory
		fakeVolume    *bfakes.FakeVolume
		fakeDB        *workerfakes.FakeVolumeFactoryDB
		fakeClock     *fakeclock.FakeClock
		logger        *lagertest.TestLogger
	)

	BeforeEach(func() {
		fakeVolume = new(bfakes.FakeVolume)
		fakeVolume.HandleReturns("some-handle")

		fakeDB = new(workerfakes.FakeVolumeFactoryDB)
		fakeClock = fakeclock.NewFakeClock(time.Unix(123, 456))
		logger = lagertest.NewTestLogger("test")

		volumeFactory = worker.NewVolumeFactory(fakeDB, fakeClock)
	})

	Context("VolumeFactory", func() {
		Describe("Build", func() {
			Context("when the volume's TTL can be found", func() {
				BeforeEach(func() {
					fakeDB.GetVolumeTTLReturns(time.Minute, true, nil)
				})

				It("releases the volume it was given", func() {
					_, found, err := volumeFactory.Build(logger, fakeVolume)
					Expect(err).ToNot(HaveOccurred())
					Expect(found).To(BeTrue())
					Expect(fakeVolume.ReleaseCallCount()).To(Equal(1))
					actualTTL := fakeVolume.ReleaseArgsForCall(0)
					Expect(actualTTL).To(BeNil())
				})

				It("embeds the original volume in the wrapped volume", func() {
					vol, found, err := volumeFactory.Build(logger, fakeVolume)
					Expect(err).ToNot(HaveOccurred())
					Expect(found).To(BeTrue())
					Expect(vol.Handle()).To(Equal("some-handle"))
				})
			})

			Context("when the volume's TTL cannot be found", func() {
				BeforeEach(func() {
					fakeDB.GetVolumeTTLReturns(0, false, nil)
				})

				It("releases the volume it was given and returns false", func() {
					_, found, err := volumeFactory.Build(logger, fakeVolume)
					Expect(err).ToNot(HaveOccurred())
					Expect(found).To(BeFalse())
					Expect(fakeVolume.ReleaseCallCount()).To(Equal(1))
					actualTTL := fakeVolume.ReleaseArgsForCall(0)
					Expect(actualTTL).To(BeNil())
				})
			})
		})
	})

	Context("Volume", func() {
		var expectedTTL time.Duration
		var expectedTTL2 time.Duration

		BeforeEach(func() {
			expectedTTL = 10 * time.Second
			expectedTTL2 = 5 * time.Second
			fakeVolume.SizeInBytesReturns(1024, nil)
			fakeDB.GetVolumeTTLReturns(expectedTTL, true, nil)
		})

		It("heartbeats", func() {
			vol, found, err := volumeFactory.Build(logger, fakeVolume)
			Expect(err).ToNot(HaveOccurred())
			Expect(found).To(BeTrue())

			By("looking up the initial ttl in the database")
			Expect(fakeDB.GetVolumeTTLCallCount()).To(Equal(1))
			actualHandle := fakeDB.GetVolumeTTLArgsForCall(0)
			Expect(actualHandle).To(Equal("some-handle"))

			By("using that ttl to heartbeat the volume initially")
			Expect(fakeVolume.SetTTLCallCount()).To(Equal(1))
			actualTTL := fakeVolume.SetTTLArgsForCall(0)
			Expect(actualTTL).To(Equal(expectedTTL))

			By("updating the volume's size in the db")
			Expect(fakeVolume.SizeInBytesCallCount()).To(Equal(1))

			By("updating the volume ttl and size in the db")
			Expect(fakeDB.SetVolumeTTLAndSizeInBytesCallCount()).To(Equal(1))
			actualHandle, actualTTL, actualVolumeSize := fakeDB.SetVolumeTTLAndSizeInBytesArgsForCall(0)
			Expect(actualHandle).To(Equal(vol.Handle()))
			Expect(actualTTL).To(Equal(expectedTTL))
			Expect(actualVolumeSize).To(Equal(int64(1024)))

			By("using the ttl from the database each tick")
			fakeDB.GetVolumeTTLReturns(expectedTTL2, true, nil)
			fakeClock.Increment(30 * time.Second)

			Eventually(fakeVolume.SetTTLCallCount).Should(Equal(2))
			actualTTL = fakeVolume.SetTTLArgsForCall(1)
			Expect(actualTTL).To(Equal(expectedTTL2))

			Eventually(fakeDB.SetVolumeTTLAndSizeInBytesCallCount).Should(Equal(2))
			actualHandle, actualTTL, actualVolumeSize = fakeDB.SetVolumeTTLAndSizeInBytesArgsForCall(1)
			Expect(actualHandle).To(Equal(vol.Handle()))
			Expect(actualTTL).To(Equal(expectedTTL2))

			By("setting ttl only when failing to get volume size")
			fakeVolume.SizeInBytesReturns(0, errors.New("an-error"))
			fakeClock.Increment(30 * time.Second)

			Eventually(fakeVolume.SetTTLCallCount).Should(Equal(3))
			actualTTL = fakeVolume.SetTTLArgsForCall(2)
			Expect(actualTTL).To(Equal(expectedTTL2))

			Eventually(fakeDB.SetVolumeTTLCallCount).Should(Equal(1))
			Eventually(fakeDB.SetVolumeTTLAndSizeInBytesCallCount).Should(Equal(2)) // did not change
			actualHandle, actualTTL = fakeDB.SetVolumeTTLArgsForCall(0)
			Expect(actualHandle).To(Equal(vol.Handle()))
			Expect(actualTTL).To(Equal(expectedTTL2))

			By("being resilient to db errors")
			fakeDB.GetVolumeTTLReturns(0, false, errors.New("disaster"))
			fakeClock.Increment(30 * time.Second)

			Eventually(fakeVolume.SetTTLCallCount).Should(Equal(4))
			actualTTL = fakeVolume.SetTTLArgsForCall(3)
			Expect(actualTTL).To(Equal(expectedTTL2))
			Eventually(fakeDB.SetVolumeTTLCallCount).Should(Equal(2))

			By("releasing the volume with a final ttl")
			vol.Release(worker.FinalTTL(2 * time.Second))
			Eventually(fakeVolume.SetTTLCallCount).Should(Equal(5))
			actualTTL = fakeVolume.SetTTLArgsForCall(4)
			Expect(actualTTL).To(Equal(2 * time.Second))

			Eventually(fakeDB.SetVolumeTTLCallCount).Should(Equal(3))
			actualHandle, actualTTL = fakeDB.SetVolumeTTLArgsForCall(2)
			Expect(actualHandle).To(Equal(vol.Handle()))
			Expect(actualTTL).To(Equal(2 * time.Second))
		})

		It("reaps the volume during heartbeat if the volume is not found", func() {
			_, found, err := volumeFactory.Build(logger, fakeVolume)
			Expect(err).ToNot(HaveOccurred())
			Expect(found).To(BeTrue())

			Expect(fakeDB.ReapVolumeCallCount()).To(Equal(0))

			fakeVolume.SetTTLReturns(baggageclaim.ErrVolumeNotFound)
			fakeClock.WaitForWatcherAndIncrement(30 * time.Second)

			Eventually(fakeDB.ReapVolumeCallCount).Should(Equal(1))
			Expect(fakeDB.ReapVolumeArgsForCall(0)).To(Equal("some-handle"))
		})
	})
})
