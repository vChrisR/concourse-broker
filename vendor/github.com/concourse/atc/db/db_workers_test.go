package db_test

import (
	"time"

	"github.com/lib/pq"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/concourse/atc"
	"github.com/concourse/atc/db"
	"github.com/concourse/atc/db/dbfakes"
)

var _ = Describe("Keeping track of workers", func() {
	var dbConn db.Conn
	var listener *pq.Listener

	var database db.DB

	var team db.SavedTeam
	BeforeEach(func() {
		postgresRunner.Truncate()

		dbConn = db.Wrap(postgresRunner.Open())
		listener = pq.NewListener(postgresRunner.DataSourceName(), time.Second, time.Minute, nil)

		Eventually(listener.Ping, 5*time.Second).ShouldNot(HaveOccurred())
		bus := db.NewNotificationsBus(listener, dbConn)

		pgxConn := postgresRunner.OpenPgx()
		fakeConnector := new(dbfakes.FakeConnector)
		retryableConn := &db.RetryableConn{Connector: fakeConnector, Conn: pgxConn}

		lockFactory := db.NewLockFactory(retryableConn)
		database = db.NewSQL(dbConn, bus, lockFactory)
	})

	AfterEach(func() {
		err := dbConn.Close()
		Expect(err).NotTo(HaveOccurred())

		err = listener.Close()
		Expect(err).NotTo(HaveOccurred())
	})

	It("can keep track of non-expired workers", func() {
		Expect(database.Workers()).To(BeEmpty())

		infoA := db.WorkerInfo{
			Name:             "workerName1",
			GardenAddr:       "1.2.3.4:7777",
			BaggageclaimURL:  "5.6.7.8:7788",
			HTTPProxyURL:     "http://example.com",
			HTTPSProxyURL:    "https://example.com",
			NoProxy:          "example.com,127.0.0.1,localhost",
			ActiveContainers: 42,
			ResourceTypes: []atc.WorkerResourceType{
				{Type: "some-resource-a", Image: "some-image-a"},
			},
			Platform:  "webos",
			Tags:      []string{"palm", "was", "great"},
			StartTime: 1461864115,
		}

		infoB := db.WorkerInfo{
			Name:             "1.2.3.4:8888",
			GardenAddr:       "1.2.3.4:8888",
			ActiveContainers: 42,
			ResourceTypes: []atc.WorkerResourceType{
				{Type: "some-resource-b", Image: "some-image-b"},
			},
			Platform:  "plan9",
			Tags:      []string{"russ", "cox", "was", "here"},
			StartTime: 1461864110,
		}
		expectedSavedWorkerA := db.SavedWorker{
			WorkerInfo: infoA,
			ExpiresIn:  0,
		}

		By("persisting workers with no TTLs")
		savedWorkerA, err := database.SaveWorker(infoA, 0)
		Expect(err).NotTo(HaveOccurred())
		expectedSavedWorkerA.Name = savedWorkerA.Name

		Expect(database.Workers()).To(ConsistOf(expectedSavedWorkerA))

		By("being idempotent")
		_, err = database.SaveWorker(infoA, 0)
		Expect(err).NotTo(HaveOccurred())

		Expect(database.Workers()).To(ConsistOf(expectedSavedWorkerA))

		By("updating attributes by name")
		infoA.GardenAddr = "1.2.3.4:9876"
		expectedSavedWorkerA.WorkerInfo = infoA

		_, err = database.SaveWorker(infoA, 0)
		Expect(err).NotTo(HaveOccurred())

		Expect(database.Workers()).To(ConsistOf(expectedSavedWorkerA))

		By("updating attributes by address")
		infoA.Name = "someNewName"
		expectedSavedWorkerA.WorkerInfo = infoA

		_, err = database.SaveWorker(infoA, 0)
		Expect(err).NotTo(HaveOccurred())

		Expect(database.Workers()).To(ConsistOf(expectedSavedWorkerA))

		By("expiring TTLs")
		ttl := 1 * time.Second

		_, err = database.SaveWorker(infoB, ttl)
		Expect(err).NotTo(HaveOccurred())

		workerInfos := func() []db.WorkerInfo {
			return getWorkerInfos(database.Workers())
		}

		Consistently(workerInfos, ttl/2).Should(ConsistOf(infoA, infoB))
		Eventually(workerInfos, 2*ttl).Should(ConsistOf(infoA))

		By("overwriting TTLs")
		_, err = database.SaveWorker(infoA, ttl)
		Expect(err).NotTo(HaveOccurred())

		Consistently(workerInfos, ttl/2).Should(ConsistOf(infoA))
		Eventually(workerInfos, 2*ttl).Should(BeEmpty())

		By("updating attributes by name with ttls")
		ttl = 1 * time.Hour
		_, err = database.SaveWorker(infoA, ttl)
		Expect(err).NotTo(HaveOccurred())
		Expect(getWorkerInfos(database.Workers())).To(ConsistOf(infoA))

		infoA.GardenAddr = "1.2.3.4:1234"

		_, err = database.SaveWorker(infoA, ttl)
		Expect(err).NotTo(HaveOccurred())

		Expect(getWorkerInfos(database.Workers())).To(ConsistOf(infoA))

		By("saving worker with the team that exists")
		team, err = database.CreateTeam(db.Team{Name: "some-team"})
		Expect(err).NotTo(HaveOccurred())

		infoA.TeamID = team.ID
		_, err = database.SaveWorker(infoA, ttl)
		Expect(err).NotTo(HaveOccurred())
		Expect(getWorkerInfos(database.Workers())).To(ConsistOf(infoA))

		By("failing to save worker with the team that does not exist")
		infoA.TeamID = 999
		_, err = database.SaveWorker(infoA, ttl)
		Expect(err).To(HaveOccurred())

		By("saving worker with no team")
		infoA.TeamID = 0
		_, err = database.SaveWorker(infoA, ttl)
		Expect(err).NotTo(HaveOccurred())
		Expect(getWorkerInfos(database.Workers())).To(ConsistOf(infoA))
	})

	It("it can keep track of a worker", func() {
		By("returning empty worker when worker doesn't exist")
		savedWorker, found, err := database.GetWorker("no-worker-here")
		Expect(err).NotTo(HaveOccurred())
		Expect(savedWorker).To(Equal(db.SavedWorker{}))
		Expect(found).To(BeFalse())

		infoA := db.WorkerInfo{
			GardenAddr:       "1.2.3.4:7777",
			BaggageclaimURL:  "http://5.6.7.8:7788",
			ActiveContainers: 42,
			ResourceTypes: []atc.WorkerResourceType{
				{Type: "some-resource-a", Image: "some-image-a"},
			},
			Platform: "webos",
			Tags:     []string{"palm", "was", "great"},
			Name:     "workerName1",
		}

		infoB := db.WorkerInfo{
			GardenAddr:       "1.2.3.4:8888",
			BaggageclaimURL:  "http://5.6.7.8:8899",
			HTTPProxyURL:     "http://example.com",
			HTTPSProxyURL:    "https://example.com",
			ActiveContainers: 42,
			ResourceTypes: []atc.WorkerResourceType{
				{Type: "some-resource-b", Image: "some-image-b"},
			},
			Platform: "plan9",
			Tags:     []string{"russ", "cox", "was", "here"},
			Name:     "workerName2",
			TeamID:   team.ID,
		}

		infoC := db.WorkerInfo{
			GardenAddr:       "1.2.3.5:8888",
			BaggageclaimURL:  "http://5.6.7.9:8899",
			ActiveContainers: 42,
			ResourceTypes: []atc.WorkerResourceType{
				{Type: "some-resource-b", Image: "some-image-b"},
			},
			Platform: "plan9",
			Tags:     []string{"russ", "cox", "was", "here"},
		}

		_, err = database.CreateTeam(db.Team{Name: "some-team"})
		Expect(err).NotTo(HaveOccurred())

		_, err = database.SaveWorker(infoA, 0)
		Expect(err).NotTo(HaveOccurred())

		savedWorkerB, err := database.SaveWorker(infoB, 0)
		Expect(err).NotTo(HaveOccurred())

		_, err = database.SaveWorker(infoC, 0)
		Expect(err).NotTo(HaveOccurred())

		By("returning one workerinfo by worker name")
		savedWorker, found, err = database.GetWorker(savedWorkerB.Name)
		Expect(err).NotTo(HaveOccurred())
		Expect(found).To(BeTrue())
		Expect(savedWorker.GardenAddr).To(Equal(infoB.GardenAddr))
		Expect(savedWorker.BaggageclaimURL).To(Equal(infoB.BaggageclaimURL))
		Expect(savedWorker.HTTPProxyURL).To(Equal(infoB.HTTPProxyURL))
		Expect(savedWorker.HTTPSProxyURL).To(Equal(infoB.HTTPSProxyURL))
		Expect(savedWorker.ActiveContainers).To(Equal(infoB.ActiveContainers))
		Expect(savedWorker.ResourceTypes).To(Equal(infoB.ResourceTypes))
		Expect(savedWorker.Platform).To(Equal(infoB.Platform))
		Expect(savedWorker.Tags).To(Equal(infoB.Tags))
		Expect(savedWorker.TeamID).To(Equal(team.ID))
		Expect(savedWorker.Name).To(Equal(infoB.Name))

		By("expiring TTLs")
		ttl := 1 * time.Second

		savedWorkerA, err := database.SaveWorker(infoA, ttl)
		Expect(err).NotTo(HaveOccurred())

		workerFound := func() bool {
			_, found, _ = database.GetWorker(savedWorkerA.Name)
			return found
		}

		Consistently(workerFound, ttl/2).Should(BeTrue())
		Eventually(workerFound, 2*ttl).Should(BeFalse())
	})
})

func getWorkerInfos(savedWorkers []db.SavedWorker, err error) []db.WorkerInfo {
	Expect(err).NotTo(HaveOccurred())
	var workerInfos []db.WorkerInfo
	for _, savedWorker := range savedWorkers {
		workerInfos = append(workerInfos, savedWorker.WorkerInfo)
	}
	return workerInfos
}
