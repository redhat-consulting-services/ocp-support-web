package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"regexp"
)

var validCaseID = regexp.MustCompile(`^[0-9]{6,10}$`)

func (h *Handler) handleUploadConfig(w http.ResponseWriter, r *http.Request) {
	configured := h.ul != nil && h.ul.IsConfigured()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"configured": configured})
}

func (h *Handler) handleUpload(w http.ResponseWriter, r *http.Request) {
	if h.ul == nil || !h.ul.IsConfigured() {
		jsonError(w, "upload not configured", 400)
		return
	}

	jobID := r.PathValue("jobId")
	if !validJobID.MatchString(jobID) {
		jsonError(w, "invalid job ID", 400)
		return
	}

	var body struct {
		CaseID       string `json:"caseID"`
		InternalUser bool   `json:"internalUser"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&body); err != nil {
		jsonError(w, "invalid request body", 400)
		return
	}

	if !validCaseID.MatchString(body.CaseID) {
		jsonError(w, "invalid case ID — must be 6-10 digits", 400)
		return
	}

	job := h.mg.GetJob(jobID)
	if job == nil {
		jsonError(w, "job not found", 404)
		return
	}
	if job.Status != "complete" {
		jsonError(w, "job is not complete", 400)
		return
	}
	if job.FilePath == "" {
		jsonError(w, "no archive file available", 400)
		return
	}

	filePath := job.FilePath

	go func() {
		if err := h.ul.Upload(body.CaseID, filePath, body.InternalUser); err != nil {
			log.Printf("Upload failed for job %s to case %s: %v", jobID, body.CaseID, err)
		}
	}()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "uploading"})
}
