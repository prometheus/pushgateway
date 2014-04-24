package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNegotiate(t *testing.T) {
	var (
		handlers = []negotiator{
			{
				"text/plain", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Write([]byte("ok"))
				}),
			},
			{
				"application/json", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Write([]byte(`{"ok":true, "version":1}`))
				}),
			},
			{
				"application/json;version=2", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Write([]byte(`{"ok":true, "version":2}`))
				}),
			},
		}

		handler = negotiate(handlers)
	)

	req, _ := http.NewRequest("GET", "/", nil)

	table := []struct {
		accept, response string
		code             int
	}{
		{"text/plain", `ok`, 200},
		{"application/json", `{"ok":true, "version":1}`, 200},
		{"application/json;version=2", `{"ok":true, "version":2}`, 200},
		{"image/jpeg", "\n", http.StatusNotAcceptable},
		{"application/json;version=2;q=0.7,application/json;q=0.6", `{"ok":true, "version":2}`, 200},
	}

	for i, tt := range table {
		req.Header.Set("Accept", tt.accept)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if expected, got := tt.code, w.Code; expected != got {
			t.Errorf("%d. expected %d, got %d", i+1, expected, got)
		}

		if expected, got := tt.response, w.Body.String(); expected != got {
			t.Errorf("%d. expected %q, got %q", i+1, expected, got)
		}
	}
}
