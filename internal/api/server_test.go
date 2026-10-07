package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"linfbbpages/internal/fbb"
)

func TestMessageVisibleTo(t *testing.T) {
	user := fbb.User{Callsign: "LU4ECL"}
	sysop := fbb.User{Callsign: "LW6DIO", Sysop: true}
	tests := []struct {
		name    string
		message fbb.Message
		user    fbb.User
		want    bool
	}{
		{name: "bulletin is visible", message: fbb.Message{Type: "B", Status: "N", From: "OTHER", To: "ALL"}, user: user, want: true},
		{name: "killed bulletin is hidden", message: fbb.Message{Type: "B", Status: "K"}, user: user, want: false},
		{name: "processing bulletin is hidden", message: fbb.Message{Type: "B", Status: "$"}, user: user, want: false},
		{name: "private from user is visible", message: fbb.Message{Type: "P", Status: "F", From: "lu4ecl-0", To: "OTHER"}, user: user, want: true},
		{name: "private to user is visible", message: fbb.Message{Type: "P", Status: "F", From: "OTHER", To: "LU4ECL"}, user: user, want: true},
		{name: "private for another user is hidden", message: fbb.Message{Type: "P", Status: "F", From: "OTHER", To: "SOMEONE"}, user: user, want: false},
		{name: "archived private for user is visible", message: fbb.Message{Type: "A", Status: "A", From: "OTHER", To: "LU4ECL"}, user: user, want: true},
		{name: "other type is hidden", message: fbb.Message{Type: "X", Status: "N", From: "LU4ECL"}, user: user, want: false},
		{name: "sysop sees killed private", message: fbb.Message{Type: "P", Status: "K", From: "OTHER", To: "SOMEONE"}, user: sysop, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := messageVisibleTo(test.message, test.user); got != test.want {
				t.Fatalf("messageVisibleTo(%+v, %+v) = %v, want %v", test.message, test.user, got, test.want)
			}
		})
	}
}

func TestListMessagesAppliesVisibilityBeforePagination(t *testing.T) {
	store, err := fbb.NewStore(apiFixtureRoot(t), fbb.Arch32Mode)
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(store, time.Hour, nil, nil)

	tests := []struct {
		name        string
		user        fbb.User
		wantTotal   int
		wantPrivate bool
	}{
		{name: "regular user", user: fbb.User{Callsign: "LU4ECL"}, wantTotal: 1033, wantPrivate: false},
		{name: "sysop", user: fbb.User{Callsign: "LW6DIO", Sysop: true}, wantTotal: 1037, wantPrivate: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/api/messages?page=2&page_size=200", nil)
			request = request.WithContext(withUser(request.Context(), test.user))
			response := httptest.NewRecorder()
			server.listMessages(response, request)
			if response.Code != http.StatusOK {
				t.Fatalf("listMessages status = %d, want 200: %s", response.Code, response.Body.String())
			}
			var result struct {
				Messages []fbb.Message `json:"messages"`
				Total    int           `json:"total"`
			}
			if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
				t.Fatal(err)
			}
			if result.Total != test.wantTotal {
				t.Fatalf("total = %d, want %d", result.Total, test.wantTotal)
			}
			foundPrivate := false
			for _, message := range result.Messages {
				if message.Type != "B" {
					foundPrivate = true
				}
			}
			if foundPrivate != test.wantPrivate {
				t.Fatalf("private messages present = %v, want %v", foundPrivate, test.wantPrivate)
			}
		})
	}
}

func TestMessageDetailHidesPrivateMessage(t *testing.T) {
	store, err := fbb.NewStore(apiFixtureRoot(t), fbb.Arch32Mode)
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(store, time.Hour, nil, nil)
	request := httptest.NewRequest(http.MethodGet, "/api/messages/789", nil)
	request = request.WithContext(withUser(request.Context(), fbb.User{Callsign: "LU4ECL"}))
	response := httptest.NewRecorder()
	server.message(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("message status = %d, want 404: %s", response.Code, response.Body.String())
	}
}

func apiFixtureRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "design", "usr", "local", "var", "ax25", "fbb")
}
