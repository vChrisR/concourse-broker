package concourse

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"

	"code.cloudfoundry.org/lager/lagertest"
	"github.com/onsi/gomega/ghttp"
	"github.com/vchrisr/concourse-broker/config"
)

func TestConcourse(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Concourse Suite")
}

var (
	atcServer *ghttp.Server
	env       config.Env
	logger    *lagertest.TestLogger
)

var _ = BeforeEach(func() {
	atcServer = ghttp.NewServer()

	env = config.Env{
		AdminUsername: "user",
		AdminPassword: "password",
		ConcourseURL:  atcServer.URL(),
	}

	logger = lagertest.NewTestLogger("concourse-broker")
})

var _ = AfterEach(func() {
	atcServer.Close()
})
