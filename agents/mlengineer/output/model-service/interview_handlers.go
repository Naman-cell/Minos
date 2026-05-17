package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

func decodeStartInterviewRequest(r *http.Request) (StartInterviewRequest, error) {
	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		return decodeMultipartStartRequest(r)
	}
	var req StartInterviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return req, fmt.Errorf("invalid json: %w", err)
	}
	return req, nil
}

func decodeMultipartStartRequest(r *http.Request) (StartInterviewRequest, error) {
	if err := r.ParseMultipartForm(20 << 20); err != nil {
		return StartInterviewRequest{}, fmt.Errorf("invalid multipart form: %w", err)
	}
	duration, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("duration_seconds")))
	req := StartInterviewRequest{
		CandidateID:     r.FormValue("candidate_id"),
		CandidateName:   r.FormValue("candidate_name"),
		ResumeText:      r.FormValue("resume_text"),
		JobDescription:  r.FormValue("job_description"),
		Seniority:       r.FormValue("seniority"),
		DurationSeconds: duration,
		Language:        r.FormValue("language"),
	}
	file, header, err := r.FormFile("resume_file")
	if err == http.ErrMissingFile {
		return req, nil
	}
	if err != nil {
		return req, fmt.Errorf("resume_file: %w", err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 15<<20))
	if err != nil {
		return req, err
	}
	resumeText, err := ExtractResumeText(header.Filename, data)
	if err != nil {
		return req, err
	}
	req.ResumeText = resumeText
	return req, nil
}
