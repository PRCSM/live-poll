package httpx

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() { gin.SetMode(gin.TestMode) }

func corsEngine() *gin.Engine {
	r := gin.New()
	r.Use(CORS("https://app.example"))
	r.GET("/x", func(c *gin.Context) { c.String(200, "ok") })
	return r
}

func TestCORS(t *testing.T) {
	r := corsEngine()

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Origin", "https://app.example")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example" {
		t.Errorf("allowed origin: got %q", got)
	}

	req = httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Origin", "https://evil.example")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("foreign origin got ACAO %q", got)
	}

	req = httptest.NewRequest(http.MethodOptions, "/x", nil)
	req.Header.Set("Origin", "https://app.example")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent || !strings.Contains(w.Header().Get("Access-Control-Allow-Headers"), "Authorization") {
		t.Errorf("preflight: code %d headers %v", w.Code, w.Header())
	}
}

func TestDecodeJSONStrict(t *testing.T) {
	type body struct {
		Name string `json:"name"`
	}
	cases := map[string]struct {
		in     string
		wantOK bool
	}{
		"valid":         {`{"name":"a"}`, true},
		"unknown field": {`{"name":"a","admin":true}`, false},
		"trailing data": {`{"name":"a"}{"name":"b"}`, false},
		"wrong type":    {`{"name":1}`, false},
		"too large":     {`{"name":"` + strings.Repeat("x", 100) + `"}`, false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			r := gin.New()
			r.Use(BodyLimit(64))
			var gotErr error
			r.POST("/", func(c *gin.Context) {
				var b body
				gotErr = DecodeJSON(c, &b)
			})
			r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.in)))
			if (gotErr == nil) != tc.wantOK {
				t.Fatalf("err = %v, wantOK %v", gotErr, tc.wantOK)
			}
		})
	}
}
