package containerserver

import (
	"code.cloudfoundry.org/lager"
	"github.com/concourse/atc/db"
	"github.com/concourse/atc/worker"
)

type Server struct {
	logger lager.Logger

	workerClient worker.Client

	db ContainerDB

	teamDBFactory db.TeamDBFactory
}

//go:generate counterfeiter . ContainerDB

type ContainerDB interface {
	GetContainer(handle string) (db.SavedContainer, bool, error)
}

func NewServer(
	logger lager.Logger,
	workerClient worker.Client,
	db ContainerDB,
	teamDBFactory db.TeamDBFactory,
) *Server {
	return &Server{
		logger:        logger,
		workerClient:  workerClient,
		db:            db,
		teamDBFactory: teamDBFactory,
	}
}
