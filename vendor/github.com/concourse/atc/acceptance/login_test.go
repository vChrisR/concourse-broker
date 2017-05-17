package acceptance_test

import (
	"fmt"
	"net/url"

	"github.com/sclevine/agouti"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/sclevine/agouti/matchers"

	"github.com/concourse/atc"
	"github.com/concourse/atc/db"
	"github.com/concourse/atc/dbng"
)

var _ = Describe("Logging In", func() {
	var atcCommand *ATCCommand
	var defaultTeam dbng.Team
	var pipelineName string
	var pipeline dbng.Pipeline

	BeforeEach(func() {
		postgresRunner.Truncate()
		dbConn = db.Wrap(postgresRunner.Open())
		dbngConn = dbng.Wrap(postgresRunner.Open())

		teamFactory := dbng.NewTeamFactory(dbngConn)
		var err error
		var found bool
		defaultTeam, found, err = teamFactory.FindTeam(atc.DefaultTeamName)
		Expect(err).NotTo(HaveOccurred())
		Expect(found).To(BeTrue()) // created by postgresRunner

		pipelineName = atc.DefaultPipelineName

		pipeline, _, err = defaultTeam.SavePipeline(pipelineName, atc.Config{
			Jobs: atc.JobConfigs{
				{Name: "job-name"},
			},
		}, dbng.ConfigVersion(1), dbng.PipelineUnpaused)
		Expect(err).NotTo(HaveOccurred())

		atcCommand = NewATCCommand(atcBin, 1, postgresRunner.DataSourceName(), []string{}, BASIC_AUTH)
		err = atcCommand.Start()
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		atcCommand.Stop()

		Expect(dbngConn.Close()).To(Succeed())
	})

	homepage := func() string {
		return atcCommand.URL("")
	}

	Describe("logging in via the UI", func() {
		Context("when user is not logged in", func() {
			var page *agouti.Page

			BeforeEach(func() {
				var err error
				page, err = agoutiDriver.NewPage()
				Expect(err).NotTo(HaveOccurred())
			})

			AfterEach(func() {
				Expect(page.Destroy()).To(Succeed())
			})

			Describe("after the user logs in", func() {
				It("should display the pipelines the user has access to in the sidebar", func() {
					Login(page, homepage())
					Expect(page.FindByClass("sidebar-toggle").Click()).To(Succeed())
					Eventually(page.FindByLink("main")).Should(BeVisible())
				})

				It("should no longer display the login link", func() {
					Eventually(page.FindByLink("login")).ShouldNot(BeFound())
				})
			})

			Context("navigating to a team specific page", func() {
				BeforeEach(func() {
					Expect(page.Navigate(atcCommand.URL("/teams/main/pipelines/main"))).To(Succeed())
				})

				It("forces a redirect to /teams/main/login with a redirect query param", func() {
					Eventually(page).Should(HaveURL(atcCommand.URL(fmt.Sprintf("/teams/main/login?redirect=%s", url.QueryEscape("/teams/main/pipelines/main")))))
				})
			})

			Context("when a build exists for an authenticated team", func() {
				var buildPath string

				BeforeEach(func() {
					// job build data
					build, err := pipeline.CreateJobBuild("job-name")
					Expect(err).NotTo(HaveOccurred())
					buildPath = fmt.Sprintf("/builds/%d", build.ID)
				})

				Context("navigating to a team specific page that exists", func() {
					BeforeEach(func() {
						Expect(page.Navigate(atcCommand.URL(buildPath))).To(Succeed())
					})

					It("forces a redirect to /login", func() {
						Eventually(page).Should(HaveURL(atcCommand.URL(fmt.Sprintf("/login?redirect=%s", url.QueryEscape(buildPath)))))
					})

					It("redirects back to the build page when user logs in", func() {
						Eventually(page.FindByLink(atc.DefaultTeamName)).Should(BeFound())
						Expect(page.FindByLink(atc.DefaultTeamName).Click()).To(Succeed())
						FillLoginFormAndSubmit(page)
						Eventually(page).Should(HaveURL(atcCommand.URL(buildPath)))
					})
				})
			})
		})
	})
})
