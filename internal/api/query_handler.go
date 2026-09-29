package api

import (
	"github.com/wyw14/solo-039/internal/journal"
	"net/http"
)

func (s *Server) snapshot(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.plant.Snapshot())
}

func (s *Server) evidence(w http.ResponseWriter, r *http.Request) {
	revision := s.plant.Campaign.Snapshot().Revision.String()
	writeJSON(w, http.StatusOK, map[string]any{"revision": revision, "proofs": s.plant.Evidence.Independent(revision)})
}

func (s *Server) recovery(w http.ResponseWriter, r *http.Request) {
	plan := journal.DefaultRecoveryPlan()
	writeJSON(w, http.StatusOK, map[string]any{"pending": plan.Pending(), "complete": plan.Complete()})
}

func (s *Server) profile(w http.ResponseWriter, r *http.Request) {
	report := s.plant.Profile.Report(40)
	if !report.GradientReady {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "profile needs at least two trays"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"readings": report.Readings, "gradient": report.Gradient, "stable": report.Stable})
}

func (s *Server) metrics(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.plant.Metrics.Snapshot())
}
