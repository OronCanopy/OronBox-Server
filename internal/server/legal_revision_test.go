package server

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLegalBundledPrivacyRevision(t *testing.T) {
	app := &App{}
	// A cached older document must not supersede the updated bundled notice.
	app.content.legal.Documents = []LegalDocument{{ID: "privacy", Revision: 2, Bodies: map[string]string{"en": "old notice"}}}
	request := httptest.NewRequest("GET", "/api/v1/meta/legal/privacy?lang=en&revision=3", nil)
	request.SetPathValue("document", "privacy")
	response := httptest.NewRecorder()
	app.handleLegalDocument(response, request)
	if response.Code != 200 || response.Header().Get("X-Legal-Revision") != "3" || !strings.Contains(response.Body.String(), "Speech recognition") {
		t.Fatalf("updated privacy notice unavailable: %d", response.Code)
	}
	request = httptest.NewRequest("GET", "/api/v1/meta/legal/privacy?revision=2", nil)
	request.SetPathValue("document", "privacy")
	response = httptest.NewRecorder()
	app.handleLegalDocument(response, request)
	if response.Code != 503 {
		t.Fatalf("stale revision unexpectedly served: %d", response.Code)
	}
}

func TestLegalNewerOnlineRevisionPreserved(t *testing.T) {
	app := &App{}
	app.content.legal.Documents = []LegalDocument{{ID: "privacy", Revision: 4, Bodies: map[string]string{"en": "new online notice"}}}
	request := httptest.NewRequest("GET", "/api/v1/meta/legal/privacy?lang=en&revision=4", nil)
	request.SetPathValue("document", "privacy")
	response := httptest.NewRecorder()
	app.handleLegalDocument(response, request)
	if response.Code != 200 || response.Header().Get("X-Legal-Revision") != "4" || response.Body.String() != "new online notice" {
		t.Fatal("newer online notice was replaced")
	}
}
