package db_test

import (
	"time"

	"github.com/concourse/atc/db"
	"github.com/concourse/atc/db/dbfakes"
	"github.com/lib/pq"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("TeamDB Volumes", func() {
	var dbConn db.Conn
	var listener *pq.Listener

	var database db.DB
	var teamDB db.TeamDB
	var otherTeamDB db.TeamDB

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
		sqlDB := db.NewSQL(dbConn, bus, lockFactory)
		database = sqlDB

		_, err := database.CreateTeam(db.Team{Name: "some-team"})
		Expect(err).NotTo(HaveOccurred())

		_, err = database.CreateTeam(db.Team{Name: "other-team"})
		Expect(err).NotTo(HaveOccurred())

		teamDBFactory := db.NewTeamDBFactory(dbConn, bus, lockFactory)
		teamDB = teamDBFactory.GetTeamDB("some-team")
		otherTeamDB = teamDBFactory.GetTeamDB("other-team")

		_, err = database.SaveWorker(db.WorkerInfo{
			Name:       "some-worker",
			GardenAddr: "1.2.3.4:7777",
		}, 10*time.Minute)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		err := dbConn.Close()
		Expect(err).NotTo(HaveOccurred())

		err = listener.Close()
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("GetVolumes", func() {
		var someTeamID int
		var otherTeamID int

		BeforeEach(func() {
			identifier := db.VolumeIdentifier{
				COW: &db.COWIdentifier{
					ParentVolumeHandle: "parent-volume-handle",
				},
			}
			someTeam, found, err := teamDB.GetTeam()
			Expect(err).NotTo(HaveOccurred())
			Expect(found).To(BeTrue())
			someTeamID = someTeam.ID

			otherTeam, found, err := otherTeamDB.GetTeam()
			Expect(err).NotTo(HaveOccurred())
			Expect(found).To(BeTrue())
			otherTeamID = otherTeam.ID

			err = database.InsertVolume(db.Volume{
				Handle:      "my-handle",
				TeamID:      someTeamID,
				WorkerName:  "some-worker",
				TTL:         5 * time.Minute,
				Identifier:  identifier,
				SizeInBytes: int64(1),
			})
			Expect(err).NotTo(HaveOccurred())

			err = database.InsertVolume(db.Volume{
				Handle:      "other-handle",
				TeamID:      otherTeamID,
				WorkerName:  "some-worker",
				TTL:         5 * time.Minute,
				Identifier:  identifier,
				SizeInBytes: int64(1),
			})
			Expect(err).NotTo(HaveOccurred())

			err = database.InsertVolume(db.Volume{
				Handle:      "resource-cache-handle",
				WorkerName:  "some-worker",
				TTL:         5 * time.Minute,
				Identifier:  identifier,
				SizeInBytes: int64(1),
			})
			Expect(err).NotTo(HaveOccurred())
		})

		It("gets only the volumes belonging to the team", func() {
			volumes, err := teamDB.GetVolumes()
			Expect(err).NotTo(HaveOccurred())
			Expect(volumes).To(HaveLen(2))
			volumeHandles := []string{volumes[0].Handle, volumes[1].Handle}
			Expect(volumeHandles).To(ConsistOf("resource-cache-handle", "my-handle"))
		})
	})
})
