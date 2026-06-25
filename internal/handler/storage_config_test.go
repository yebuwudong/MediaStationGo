package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestStorageConfigHandlersRejectQuark(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.PUT("/admin/storage/:type", saveStorageConfigHandler(nil))

	req := httptest.NewRequest(http.MethodPut, "/admin/storage/quark", strings.NewReader(`{"type":"quark","config":{"cookie":"x"}}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s, want 400", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "unsupported storage type") {
		t.Fatalf("body = %s, want unsupported storage type", w.Body.String())
	}
}
