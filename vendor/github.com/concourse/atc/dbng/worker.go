package dbng

import (
	"errors"
	"time"

	"github.com/concourse/atc"
)

type WorkerState string

const (
	WorkerStateRunning  = WorkerState("running")
	WorkerStateStalled  = WorkerState("stalled")
	WorkerStateLanding  = WorkerState("landing")
	WorkerStateLanded   = WorkerState("landed")
	WorkerStateRetiring = WorkerState("retiring")
)

var (
	ErrWorkerNotPresent         = errors.New("worker-not-present-in-db")
	ErrCannotPruneRunningWorker = errors.New("worker-not-stalled-for-pruning")
)

type Worker struct {
	Name       string
	GardenAddr *string
	State      WorkerState

	BaggageclaimURL *string
	HTTPProxyURL    string
	HTTPSProxyURL   string
	NoProxy         string

	ActiveContainers int
	ResourceTypes    []atc.WorkerResourceType
	Platform         string
	Tags             []string
	TeamID           int
	StartTime        int64

	TeamName  string
	ExpiresIn time.Duration
}
