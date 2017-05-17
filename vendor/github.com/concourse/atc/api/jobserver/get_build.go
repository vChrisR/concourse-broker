package jobserver

import (
	"encoding/json"
	"net/http"

	"github.com/concourse/atc/api/present"
	"github.com/concourse/atc/db"
)

func (s *Server) GetJobBuild(pipelineDB db.PipelineDB) http.Handler {
	logger := s.logger.Session("get-job-build")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jobName := r.FormValue(":job_name")
		buildName := r.FormValue(":build_name")

		build, found, err := pipelineDB.GetJobBuild(jobName, buildName)
		if err != nil {
			logger.Error("failed-to-get-job-build", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if !found {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		w.WriteHeader(http.StatusOK)

		json.NewEncoder(w).Encode(present.Build(build))
	})
}
