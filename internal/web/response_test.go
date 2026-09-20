package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestWriteJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	payload := map[string]string{"foo": "bar"}
	writeJSON(rec, http.StatusCreated, payload)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var got map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["foo"] != "bar" {
		t.Errorf("got foo=%q, want %q", got["foo"], "bar")
	}
}

func TestWriteError(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, http.StatusBadRequest, "bad request parameter")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	var got map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["error"] != "bad request parameter" {
		t.Errorf("error = %q, want %q", got["error"], "bad request parameter")
	}
}

func TestDecodeJSON(t *testing.T) {
	t.Run("valid body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewReader([]byte(`{"ticker":"AAPL","shares":10}`)))
		rec := httptest.NewRecorder()

		var body struct {
			Ticker string  `json:"ticker"`
			Shares float64 `json:"shares"`
		}
		ok := decodeJSON(rec, req, &body)
		if !ok {
			t.Fatal("decodeJSON returned false for valid body")
		}
		if body.Ticker != "AAPL" || body.Shares != 10 {
			t.Errorf("decoded %+v, want AAPL / 10", body)
		}
	})

	t.Run("invalid body writes 400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewReader([]byte(`invalid json`)))
		rec := httptest.NewRecorder()

		var body struct {
			Ticker string `json:"ticker"`
		}
		ok := decodeJSON(rec, req, &body)
		if ok {
			t.Fatal("decodeJSON returned true for invalid body")
		}
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", rec.Code)
		}
		var errResp map[string]string
		if err := json.NewDecoder(rec.Body).Decode(&errResp); err != nil {
			t.Fatalf("decode error response: %v", err)
		}
		if errResp["error"] != "invalid request body" {
			t.Errorf("error = %q, want %q", errResp["error"], "invalid request body")
		}
	})
}

func TestWriteAPIOK(t *testing.T) {
	rec := httptest.NewRecorder()
	now := time.Now().Unix()
	writeAPIOK(rec, map[string]string{"message": "pong"})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var resp apiResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Success {
		t.Errorf("success = false, want true")
	}
	if resp.Error != "" {
		t.Errorf("error = %q, want empty", resp.Error)
	}
	if resp.Timestamp < now {
		t.Errorf("timestamp = %d, want >= %d", resp.Timestamp, now)
	}
	dataMap, ok := resp.Data.(map[string]any)
	if !ok || dataMap["message"] != "pong" {
		t.Errorf("data = %+v, want {message: pong}", resp.Data)
	}
}

func TestWriteAPIResponse(t *testing.T) {
	rec := httptest.NewRecorder()
	writeAPIResponse(rec, http.StatusAccepted, map[string]string{"status": "queued"})

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", rec.Code)
	}
	var resp apiResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Success {
		t.Errorf("success = false, want true")
	}
}

func TestWriteAPIError(t *testing.T) {
	rec := httptest.NewRecorder()
	writeAPIError(rec, http.StatusUnauthorized, "unauthorized access")

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	var resp apiResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Success {
		t.Errorf("success = true, want false")
	}
	if resp.Error != "unauthorized access" {
		t.Errorf("error = %q, want %q", resp.Error, "unauthorized access")
	}
	if resp.Data != nil {
		t.Errorf("data = %+v, want nil", resp.Data)
	}
}

func TestDecodeAPIJSON(t *testing.T) {
	t.Run("valid body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewReader([]byte(`{"foo":"bar"}`)))
		rec := httptest.NewRecorder()

		var body struct {
			Foo string `json:"foo"`
		}
		ok := decodeAPIJSON(rec, req, &body)
		if !ok {
			t.Fatal("decodeAPIJSON returned false for valid body")
		}
		if body.Foo != "bar" {
			t.Errorf("foo = %q, want bar", body.Foo)
		}
	})

	t.Run("invalid body writes envelope 400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewReader([]byte(`not json`)))
		rec := httptest.NewRecorder()

		var body struct {
			Foo string `json:"foo"`
		}
		ok := decodeAPIJSON(rec, req, &body)
		if ok {
			t.Fatal("decodeAPIJSON returned true for invalid body")
		}
		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", rec.Code)
		}
		var resp apiResponse
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("decode error response: %v", err)
		}
		if resp.Success {
			t.Errorf("success = true, want false")
		}
		if resp.Error != "invalid request body" {
			t.Errorf("error = %q, want %q", resp.Error, "invalid request body")
		}
	})
}
