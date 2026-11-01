package pkg

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestJSON_SetsContentTypeAndEncodesBody(t *testing.T) {
	rr := httptest.NewRecorder()

	JSON(rr, map[string]any{
		"status":   http.StatusOK,
		"response": "ok",
	})

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	if got := rr.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("expected content-type application/json, got %q", got)
	}

	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode json body: %v", err)
	}

	if body["response"] != "ok" {
		t.Fatalf("expected response body field to be %q, got %#v", "ok", body["response"])
	}
}

func TestJSON_EncodeFailureWritesFallbackError(t *testing.T) {
	rr := httptest.NewRecorder()

	JSON(rr, map[string]any{
		"invalid": make(chan int),
	})

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, rr.Code)
	}

	if got := rr.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("expected content-type application/json, got %q", got)
	}

	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode fallback json body: %v", err)
	}

	if body["status"] != float64(http.StatusInternalServerError) {
		t.Fatalf("expected fallback status field to be %d, got %#v", http.StatusInternalServerError, body["status"])
	}

	if body["response"] != "Error encoding JSON" {
		t.Fatalf("expected fallback response field to be %q, got %#v", "Error encoding JSON", body["response"])
	}
}

func TestJSON_WriterFailureReturnsWithoutRecursion(t *testing.T) {
	rw := &alwaysFailWriter{header: make(http.Header)}
	done := make(chan struct{})

	go func() {
		JSON(rw, map[string]any{"status": "ok"})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("JSON did not return in time")
	}

	if len(rw.statusCodes) != 1 || rw.statusCodes[0] != http.StatusInternalServerError {
		t.Fatalf("expected one status code write of %d, got %#v", http.StatusInternalServerError, rw.statusCodes)
	}

	if rw.writeCalls != 2 {
		t.Fatalf("expected two write attempts (initial + fallback), got %d", rw.writeCalls)
	}
}

type alwaysFailWriter struct {
	header      http.Header
	statusCodes []int
	writeCalls  int
}

func (w *alwaysFailWriter) Header() http.Header {
	return w.header
}

func (w *alwaysFailWriter) WriteHeader(statusCode int) {
	w.statusCodes = append(w.statusCodes, statusCode)
}

func (w *alwaysFailWriter) Write(p []byte) (int, error) {
	w.writeCalls++
	return 0, errors.New("write failed")
}
