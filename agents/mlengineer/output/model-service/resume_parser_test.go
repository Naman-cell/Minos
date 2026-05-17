package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExtractDOCXResumeText(t *testing.T) {
	data := makeDOCX(t, "Senior backend engineer with Redis and API reliability.")
	got, err := ExtractResumeText("resume.docx", data)
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, got, "Senior backend engineer")
	assertContains(t, got, "Redis")
}

func TestExtractSimplePDFResumeText(t *testing.T) {
	data := []byte("%PDF-1.4\n1 0 obj <<>> stream\nBT (Backend engineer with Redis caching and incident response) Tj ET\nendstream\nendobj\n%%EOF")
	got, err := ExtractResumeText("resume.pdf", data)
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, got, "Backend engineer")
	assertContains(t, got, "Redis caching")
}

func TestStartInterviewAcceptsMultipartResumeFile(t *testing.T) {
	server := NewServer(mustBrainA(t), NewBrainB(), NewMockBrainC(), NewStateMachine())
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("candidate_id", "multipart_001")
	_ = writer.WriteField("job_description", "Senior backend role requiring API reliability.")
	_ = writer.WriteField("seniority", "senior")
	part, err := writer.CreateFormFile("resume_file", "resume.docx")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(makeDOCX(t, "Resume says Redis caching and Kafka ownership.")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/interviews", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	server.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var decoded CreateInterviewResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.CandidateID != "multipart_001" {
		t.Fatalf("candidate_id=%q", decoded.CandidateID)
	}
	session, ok := server.sessions.Find("multipart_001")
	if !ok {
		t.Fatal("session missing")
	}
	assertContains(t, session.ResumeText, "Redis caching")
	assertContains(t, session.SystemPrompt(), "Kafka ownership")
}

func makeDOCX(t *testing.T, text string) []byte {
	t.Helper()
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	file, err := writer.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	escaped := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(text)
	_, _ = file.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>` + escaped + `</w:t></w:r></w:p></w:body></w:document>`))
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func mustBrainA(t *testing.T) *BrainA {
	t.Helper()
	brainA, err := NewBrainA(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = brainA.Close()
	})
	return brainA
}
