package image_test

import (
	"archive/tar"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"

	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/lager/lagertest"
	"github.com/concourse/atc"
	"github.com/concourse/atc/db"
	"github.com/concourse/atc/resource"
	rfakes "github.com/concourse/atc/resource/resourcefakes"
	"github.com/concourse/atc/worker"
	"github.com/concourse/atc/worker/image"
	wfakes "github.com/concourse/atc/worker/workerfakes"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("Image", func() {
	var fakeTrackerFactory *rfakes.FakeTrackerFactory
	var fakeImageTracker *rfakes.FakeTracker
	var fakeResourceFetcherFactory *rfakes.FakeFetcherFactory
	var fakeResourceFetcher *rfakes.FakeFetcher

	var fetchedImage worker.Image

	var stderrBuf *gbytes.Buffer

	var logger lager.Logger
	var imageResource atc.ImageResource
	var signals chan os.Signal
	var identifier worker.Identifier
	var metadata worker.Metadata
	var fakeImageFetchingDelegate *wfakes.FakeImageFetchingDelegate
	var fakeWorker *wfakes.FakeClient
	var customTypes atc.ResourceTypes
	var privileged bool

	var fetchedVolume worker.Volume
	var fetchedMetadataReader io.ReadCloser
	var fetchedVersion atc.Version
	var fetchErr error
	var teamID int

	BeforeEach(func() {
		fakeTrackerFactory = new(rfakes.FakeTrackerFactory)

		fakeImageTracker = new(rfakes.FakeTracker)
		fakeTrackerFactory.TrackerForReturns(fakeImageTracker)

		fakeResourceFetcherFactory = new(rfakes.FakeFetcherFactory)
		fakeResourceFetcher = new(rfakes.FakeFetcher)
		fakeResourceFetcherFactory.FetcherForReturns(fakeResourceFetcher)

		stderrBuf = gbytes.NewBuffer()

		logger = lagertest.NewTestLogger("test")
		imageResource = atc.ImageResource{
			Type:   "docker",
			Source: atc.Source{"some": "source"},
		}
		signals = make(chan os.Signal)
		identifier = worker.Identifier{
			BuildID: 1234,
			PlanID:  "some-plan-id",
		}
		metadata = worker.Metadata{
			PipelineName:         "some-pipeline",
			Type:                 db.ContainerTypeCheck,
			StepName:             "some-step",
			WorkingDirectory:     "some-working-dir",
			EnvironmentVariables: []string{"some", "env", "var"},
		}
		fakeImageFetchingDelegate = new(wfakes.FakeImageFetchingDelegate)
		fakeImageFetchingDelegate.StderrReturns(stderrBuf)
		fakeWorker = new(wfakes.FakeClient)
		teamID = 123
		customTypes = atc.ResourceTypes{
			{
				Name:   "custom-type-a",
				Type:   "base-type",
				Source: atc.Source{"some": "source"},
			},
			{
				Name:   "custom-type-b",
				Type:   "custom-type-a",
				Source: atc.Source{"some": "source"},
			},
		}
	})

	JustBeforeEach(func() {
		imageFactory := image.NewFactory(
			fakeTrackerFactory,
			fakeResourceFetcherFactory,
		)

		fetchedImage = imageFactory.NewImage(
			logger,
			signals,
			imageResource,
			identifier,
			metadata,
			atc.Tags{"worker", "tags"},
			teamID,
			customTypes,
			fakeWorker,
			fakeImageFetchingDelegate,
			privileged,
		)

		fetchedVolume, fetchedMetadataReader, fetchedVersion, fetchErr = fetchedImage.Fetch()
	})

	Context("when initializing the Check resource works", func() {
		var (
			fakeCheckResource *rfakes.FakeResource
		)

		BeforeEach(func() {
			fakeCheckResource = new(rfakes.FakeResource)
			fakeImageTracker.InitReturns(fakeCheckResource, nil)
		})

		Context("when check returns a version", func() {
			BeforeEach(func() {
				fakeCheckResource.CheckReturns([]atc.Version{{"v": "1"}}, nil)
			})

			Context("when saving the version in the database succeeds", func() {
				BeforeEach(func() {
					fakeImageFetchingDelegate.ImageVersionDeterminedReturns(nil)
				})

				Context("when fetching resource fails", func() {
					BeforeEach(func() {
						fakeResourceFetcher.FetchReturns(nil, resource.ErrInterrupted)
					})

					It("returns error", func() {
						Expect(fetchErr).To(Equal(resource.ErrInterrupted))
					})
				})

				Context("when fetching resource succeeds", func() {
					var (
						fakeFetchSource     *rfakes.FakeFetchSource
						fakeVersionedSource *rfakes.FakeVersionedSource
					)

					BeforeEach(func() {
						fakeFetchSource = new(rfakes.FakeFetchSource)
						fakeResourceFetcher.FetchReturns(fakeFetchSource, nil)

						fakeVersionedSource = new(rfakes.FakeVersionedSource)
						fakeVersionedSource.StreamOutReturns(tarStreamWith("some-tar-contents"), nil)
						fakeVolume := new(wfakes.FakeVolume)
						fakeVersionedSource.VolumeReturns(fakeVolume)
						fakeFetchSource.VersionedSourceReturns(fakeVersionedSource)
					})

					Context("when the resource has a volume", func() {
						var (
							fakeVolume *wfakes.FakeVolume
							volumePath string
						)

						BeforeEach(func() {
							fakeVolume = new(wfakes.FakeVolume)
							volumePath = "C:/Documents and Settings/Evan/My Documents"

							fakeVolume.PathReturns(volumePath)
							fakeVersionedSource.VolumeReturns(fakeVolume)

							privileged = true
						})

						It("creates a cow volume with the resource's volume", func() {
							Expect(fakeWorker.CreateVolumeCallCount()).To(Equal(1))
							_, actualVolumeSpec, actualTeamID := fakeWorker.CreateVolumeArgsForCall(0)
							Expect(actualVolumeSpec).To(Equal(worker.VolumeSpec{
								Strategy: worker.ContainerRootFSStrategy{
									Parent: fakeVolume,
								},
								Privileged: true,
								TTL:        worker.ContainerTTL,
							}))
							Expect(actualTeamID).To(Equal(teamID))
						})

						Context("when creating the cow volume fails", func() {
							var err error
							BeforeEach(func() {
								err = errors.New("create-volume-err")
								fakeWorker.CreateVolumeReturns(nil, err)
							})

							It("returns an error", func() {
								Expect(fetchErr).To(Equal(err))
							})
						})

						Context("when creating the cow volume succeeds", func() {
							var fakeCOWVolume *wfakes.FakeVolume
							BeforeEach(func() {
								fakeCOWVolume = new(wfakes.FakeVolume)
								fakeWorker.CreateVolumeReturns(fakeCOWVolume, nil)

								fakeWorker.CreateVolumeStub = func(lager.Logger, worker.VolumeSpec, int) (worker.Volume, error) {
									Expect(fakeVolume.ReleaseCallCount()).To(Equal(0))
									return fakeCOWVolume, nil
								}
							})

							It("returns the COWVolume as the image volume", func() {
								Expect(fetchedVolume).To(Equal(fakeCOWVolume))
							})
						})

						It("succeeds", func() {
							Expect(fetchErr).To(BeNil())
						})

						It("calls StreamOut on the versioned source with the right metadata path", func() {
							Expect(fakeVersionedSource.StreamOutCallCount()).To(Equal(1))
							Expect(fakeVersionedSource.StreamOutArgsForCall(0)).To(Equal("metadata.json"))
						})

						It("returns a tar stream containing the contents of metadata.json", func() {
							Expect(ioutil.ReadAll(fetchedMetadataReader)).To(Equal([]byte("some-tar-contents")))
						})

						It("closing the tar stream releases the fetch source", func() {
							Expect(fakeFetchSource.ReleaseCallCount()).To(Equal(0))
							fetchedMetadataReader.Close()
							Expect(fakeFetchSource.ReleaseCallCount()).To(Equal(1))
						})

						It("has the version on the image", func() {
							Expect(fetchedVersion).To(Equal(atc.Version{"v": "1"}))
						})

						It("creates a tracker for checking and getting the image resource", func() {
							Expect(fakeTrackerFactory.TrackerForCallCount()).To(Equal(1))
							Expect(fakeTrackerFactory.TrackerForArgsForCall(0)).To(Equal(fakeWorker))
						})

						It("created the 'check' resource with the correct session, with the currently fetching type removed from the set", func() {
							Expect(fakeImageTracker.InitCallCount()).To(Equal(1))
							_, metadata, session, resourceType, tags, actualTeamID, actualCustomTypes, delegate := fakeImageTracker.InitArgsForCall(0)
							Expect(metadata).To(Equal(resource.EmptyMetadata{}))
							Expect(session).To(Equal(resource.Session{
								ID: worker.Identifier{
									BuildID:             1234,
									PlanID:              "some-plan-id",
									ImageResourceType:   "docker",
									ImageResourceSource: atc.Source{"some": "source"},
									Stage:               db.ContainerStageCheck,
								},
								Metadata: worker.Metadata{
									PipelineName:         "some-pipeline",
									Type:                 db.ContainerTypeCheck,
									StepName:             "some-step",
									WorkingDirectory:     "",  // figure this out once we actually support hijacking these
									EnvironmentVariables: nil, // figure this out once we actually support hijacking these
								},
							}))
							Expect(resourceType).To(Equal(resource.ResourceType("docker")))
							Expect(tags).To(Equal(atc.Tags{"worker", "tags"}))
							Expect(actualTeamID).To(Equal(teamID))
							Expect(actualCustomTypes).To(Equal(customTypes))
							Expect(delegate).To(Equal(fakeImageFetchingDelegate))
						})

						It("ran 'check' with the right config", func() {
							Expect(fakeCheckResource.CheckCallCount()).To(Equal(1))
							checkSource, checkVersion := fakeCheckResource.CheckArgsForCall(0)
							Expect(checkVersion).To(BeNil())
							Expect(checkSource).To(Equal(imageResource.Source))
						})

						It("saved the image resource version in the database", func() {
							expectedIdentifier := worker.VolumeIdentifier{
								ResourceCache: &db.ResourceCacheIdentifier{
									ResourceVersion: atc.Version{"v": "1"},
									ResourceHash:    `docker{"some":"source"}`,
								},
							}
							Expect(fakeImageFetchingDelegate.ImageVersionDeterminedCallCount()).To(Equal(1))
							Expect(fakeImageFetchingDelegate.ImageVersionDeterminedArgsForCall(0)).To(Equal(expectedIdentifier))
						})

						It("releases the check resource, which includes releasing its volume", func() {
							Expect(fakeCheckResource.ReleaseCallCount()).To(Equal(1))
						})

						It("fetches resource with correct session", func() {
							Expect(fakeResourceFetcher.FetchCallCount()).To(Equal(1))
							_, session, tags, actualTeamID, actualCustomTypes, cacheID, metadata, delegate, resourceOptions, _, _ := fakeResourceFetcher.FetchArgsForCall(0)
							Expect(metadata).To(Equal(resource.EmptyMetadata{}))
							Expect(session).To(Equal(resource.Session{
								ID: worker.Identifier{
									BuildID:             1234,
									PlanID:              "some-plan-id",
									ImageResourceType:   "docker",
									ImageResourceSource: atc.Source{"some": "source"},
									Stage:               db.ContainerStageGet,
								},
								Metadata: worker.Metadata{
									PipelineName:         "some-pipeline",
									Type:                 db.ContainerTypeGet,
									StepName:             "some-step",
									WorkingDirectory:     "",  // figure this out once we actually support hijacking these
									EnvironmentVariables: nil, // figure this out once we actually support hijacking these
								},
							}))
							Expect(tags).To(Equal(atc.Tags{"worker", "tags"}))
							Expect(actualTeamID).To(Equal(teamID))
							Expect(cacheID).To(Equal(resource.ResourceCacheIdentifier{
								Type:    "docker",
								Version: atc.Version{"v": "1"},
								Source:  atc.Source{"some": "source"},
							}))
							Expect(actualCustomTypes).To(Equal(customTypes))
							Expect(delegate).To(Equal(fakeImageFetchingDelegate))
							Expect(resourceOptions.ResourceType()).To(Equal(resource.ResourceType("docker")))
							expectedLockName := fmt.Sprintf("%x",
								sha256.Sum256([]byte(
									`{"type":"docker","version":{"v":"1"},"source":{"some":"source"},"worker_name":"fake-worker-name"}`,
								)),
							)
							Expect(resourceOptions.LockName("fake-worker-name")).To(Equal(expectedLockName))
						})

						It("gets the volume", func() {
							Expect(fakeVersionedSource.VolumeCallCount()).To(Equal(1))
						})

						Context("when streaming the metadata out fails", func() {
							disaster := errors.New("nope")

							BeforeEach(func() {
								fakeVersionedSource.StreamOutReturns(nil, disaster)
							})

							It("returns the error", func() {
								Expect(fetchErr).To(Equal(disaster))
							})
						})

						Context("when the resource still does not have a volume for some reason", func() {
							BeforeEach(func() {
								fakeVersionedSource.VolumeReturns(nil)
							})

							It("returns an appropriate error", func() {
								Expect(fetchErr).To(Equal(image.ErrImageGetDidNotProduceVolume))
							})
						})
					})
				})
			})

			Context("when saving the version in the database fails", func() {
				var imageVersionSavingCalamity error
				BeforeEach(func() {
					imageVersionSavingCalamity = errors.New("hang in there bud")
					fakeImageFetchingDelegate.ImageVersionDeterminedReturns(imageVersionSavingCalamity)
				})

				It("returns the error", func() {
					Expect(fetchErr).To(Equal(imageVersionSavingCalamity))
				})

				It("does not construct the 'get' resource", func() {
					Expect(fakeResourceFetcher.FetchCallCount()).To(Equal(0))
				})
			})
		})

		Context("when check returns no versions", func() {
			BeforeEach(func() {
				fakeCheckResource.CheckReturns([]atc.Version{}, nil)
			})

			It("exits with ErrImageUnavailable", func() {
				Expect(fetchErr).To(Equal(image.ErrImageUnavailable))
			})

			It("does not attempt to save any versions in the database", func() {
				Expect(fakeImageFetchingDelegate.ImageVersionDeterminedCallCount()).To(Equal(0))
			})
		})

		Context("when check returns an error", func() {
			var (
				disaster error
			)

			BeforeEach(func() {
				disaster = errors.New("wah")
				fakeCheckResource.CheckReturns(nil, disaster)
			})

			It("returns the error", func() {
				Expect(fetchErr).To(Equal(disaster))
			})

			It("does not construct the 'get' resource", func() {
				Expect(fakeResourceFetcher.FetchCallCount()).To(Equal(0))
			})
		})
	})

	Context("when initializing the Check resource fails", func() {
		var (
			disaster error
		)

		BeforeEach(func() {
			disaster = errors.New("wah")
			fakeImageTracker.InitReturns(nil, disaster)
		})

		It("returns the error", func() {
			Expect(fetchErr).To(Equal(disaster))
		})

		It("does not construct the 'get' resource", func() {
			Expect(fakeResourceFetcher.FetchCallCount()).To(Equal(0))
		})
	})
})

func tarStreamWith(metadata string) io.ReadCloser {
	buffer := gbytes.NewBuffer()

	tarWriter := tar.NewWriter(buffer)
	err := tarWriter.WriteHeader(&tar.Header{
		Name: "metadata.json",
		Mode: 0600,
		Size: int64(len(metadata)),
	})
	Expect(err).NotTo(HaveOccurred())

	_, err = tarWriter.Write([]byte(metadata))
	Expect(err).NotTo(HaveOccurred())

	err = tarWriter.Close()
	Expect(err).NotTo(HaveOccurred())

	return buffer
}
