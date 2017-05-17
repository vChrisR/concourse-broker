package acceptance_test

import (
	"bytes"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/lib/pq"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/concourse/atc"
	"github.com/concourse/atc/db"
	"github.com/concourse/atc/db/dbfakes"
)

var _ = Describe("Multiple ATCs", func() {
	var atcOneCommand *ATCCommand
	var atcTwoCommand *ATCCommand

	var dbListener *pq.Listener

	BeforeEach(func() {
		postgresRunner.Truncate()
		dbConn = db.Wrap(postgresRunner.Open())
		dbListener = pq.NewListener(postgresRunner.DataSourceName(), time.Second, time.Minute, nil)
		bus := db.NewNotificationsBus(dbListener, dbConn)

		pgxConn := postgresRunner.OpenPgx()
		fakeConnector := new(dbfakes.FakeConnector)
		retryableConn := &db.RetryableConn{Connector: fakeConnector, Conn: pgxConn}

		lockFactory := db.NewLockFactory(retryableConn)
		sqlDB = db.NewSQL(dbConn, bus, lockFactory)

		atcOneCommand = NewATCCommand(atcBin, 1, postgresRunner.DataSourceName(), []string{}, NO_AUTH)
		err := atcOneCommand.Start()
		Expect(err).NotTo(HaveOccurred())

		atcTwoCommand = NewATCCommand(atcBin, 2, postgresRunner.DataSourceName(), []string{}, NO_AUTH)
		err = atcTwoCommand.Start()
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		atcOneCommand.Stop()
		atcTwoCommand.Stop()

		Expect(dbConn.Close()).To(Succeed())
		Expect(dbListener.Close()).To(Succeed())
	})

	Describe("Pipes", func() {
		var client *http.Client
		BeforeEach(func() {
			client = &http.Client{
				Transport: &http.Transport{},
			}
		})

		addAuthorization := func(originalRequest *http.Request, atcCommand *ATCCommand) {
			request, err := http.NewRequest("GET", atcCommand.URL("/api/v1/teams/main/auth/token"), nil)
			resp, err := client.Do(request)
			Expect(err).NotTo(HaveOccurred())

			defer resp.Body.Close()
			var atcToken atc.AuthToken
			body, err := ioutil.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())

			err = json.Unmarshal(body, &atcToken)
			Expect(err).NotTo(HaveOccurred())

			originalRequest.Header.Add("Authorization", atcToken.Type+" "+atcToken.Value)
		}

		createPipe := func(atcCommand *ATCCommand) atc.Pipe {
			req, err := http.NewRequest("POST", atcCommand.URL("/api/v1/pipes"), nil)
			Expect(err).NotTo(HaveOccurred())
			addAuthorization(req, atcCommand)

			response, err := client.Do(req)
			Expect(err).NotTo(HaveOccurred())

			Expect(response.StatusCode).To(Equal(http.StatusCreated))

			var pipe atc.Pipe
			err = json.NewDecoder(response.Body).Decode(&pipe)
			Expect(err).NotTo(HaveOccurred())

			return pipe
		}

		readPipe := func(id string, atcCommand *ATCCommand) *http.Response {
			req, err := http.NewRequest("GET", atcCommand.URL("/api/v1/pipes/"+id), nil)
			Expect(err).NotTo(HaveOccurred())
			addAuthorization(req, atcCommand)

			response, err := client.Do(req)
			Expect(err).NotTo(HaveOccurred())

			return response
		}

		writePipe := func(id string, body io.Reader, atcCommand *ATCCommand) *http.Response {
			req, err := http.NewRequest("PUT", atcCommand.URL("/api/v1/pipes/"+id), body)
			Expect(err).NotTo(HaveOccurred())
			addAuthorization(req, atcCommand)

			response, err := client.Do(req)
			Expect(err).NotTo(HaveOccurred())

			return response
		}

		It("data can be written or read from the pipe regardless of where it was created", func() {
			pipe := createPipe(atcOneCommand)

			readRes := readPipe(pipe.ID, atcOneCommand)
			Expect(readRes.StatusCode).To(Equal(http.StatusOK))

			writeRes := writePipe(pipe.ID, bytes.NewBufferString("some data"), atcOneCommand)
			Expect(writeRes.StatusCode).To(Equal(http.StatusOK))

			Expect(ioutil.ReadAll(readRes.Body)).To(Equal([]byte("some data")))
			Eventually(func() int {
				secondReadRes := readPipe(pipe.ID, atcOneCommand)
				defer secondReadRes.Body.Close()

				return secondReadRes.StatusCode
			}).Should(Equal(http.StatusNotFound))

			readRes.Body.Close()
			writeRes.Body.Close()

			pipe = createPipe(atcOneCommand)

			readRes = readPipe(pipe.ID, atcOneCommand)
			Expect(readRes.StatusCode).To(Equal(http.StatusOK))

			writeRes = writePipe(pipe.ID, bytes.NewBufferString("some data"), atcTwoCommand)
			Expect(writeRes.StatusCode).To(Equal(http.StatusOK))

			Expect(ioutil.ReadAll(readRes.Body)).To(Equal([]byte("some data")))
			Eventually(func() int {
				secondReadRes := readPipe(pipe.ID, atcOneCommand)
				defer secondReadRes.Body.Close()

				return secondReadRes.StatusCode
			}).Should(Equal(http.StatusNotFound))

			readRes.Body.Close()
			writeRes.Body.Close()

			pipe = createPipe(atcTwoCommand)
			readRes = readPipe(pipe.ID, atcOneCommand)
			Expect(readRes.StatusCode).To(Equal(http.StatusOK))

			writeRes = writePipe(pipe.ID, bytes.NewBufferString("some kind of data"), atcTwoCommand)
			Expect(writeRes.StatusCode).To(Equal(http.StatusOK))
			Expect(ioutil.ReadAll(readRes.Body)).To(Equal([]byte("some kind of data")))

			readRes.Body.Close()
			writeRes.Body.Close()

			pipe = createPipe(atcOneCommand)

			readRes = readPipe(pipe.ID, atcTwoCommand)
			Expect(readRes.StatusCode).To(Equal(http.StatusOK))

			writeRes = writePipe(pipe.ID, bytes.NewBufferString("some other data"), atcTwoCommand)
			Expect(writeRes.StatusCode).To(Equal(http.StatusOK))

			Expect(ioutil.ReadAll(readRes.Body)).To(Equal([]byte("some other data")))

			readRes.Body.Close()
			writeRes.Body.Close()
		})
	})
})
