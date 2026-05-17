package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOfferReturnsSDP(t *testing.T) {
	relay := NewRelay(NewSTTClient(), &fakeModel{})
	req := httptest.NewRequest(http.MethodGet, "/offer", nil)
	rec := httptest.NewRecorder()

	relay.handleOffer(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp SignalResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.SDP, "v=0") {
		t.Fatalf("missing SDP offer: %#v", resp)
	}
}

func TestAnswerRequiresOffer(t *testing.T) {
	relay := NewRelay(NewSTTClient(), &fakeModel{})
	req := httptest.NewRequest(http.MethodPost, "/answer", strings.NewReader(`{"sdp":"v=0"}`))
	rec := httptest.NewRecorder()

	relay.handleAnswer(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}
