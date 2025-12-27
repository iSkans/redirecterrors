package redirecterrors_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/iskans/redirecterrors"
)

func TestBadConfig(t *testing.T) {
	cfg := redirecterrors.CreateConfig()
	cfg.Status = []string{}
	cfg.Target = ""

	ctx := context.Background()
	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {})

	_, err := redirecterrors.New(ctx, next, cfg, "redirecterrors-plugin")
	if !assert(t, err != nil) {
		return
	}
	assert(t, err.Error() == "target url must be set")
}

// TODO: more tests: config parsing & non-intercepted response
func TestRedirect(t *testing.T) {
	cfg := redirecterrors.CreateConfig()
	cfg.Status = []string{"401", "402"}
	cfg.Target = "http://target/?status={status}&url={url}"

	ctx := context.Background()
	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) { rw.WriteHeader(401) })

	handler, err := redirecterrors.New(ctx, next, cfg, "redirecterrors-plugin")
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost", nil)
	if err != nil {
		t.Fatal(err)
	}

	handler.ServeHTTP(recorder, req)

	resp := recorder.Result()
	assertHeader(t, resp, "Location", "http://target/?status=401&url=http%3A%2F%2Flocalhost")
	assertCode(t, resp, 302)
}

func TestNoRedirect(t *testing.T) {
	cfg := redirecterrors.CreateConfig()
	cfg.Status = []string{}
	cfg.Target = "http://target/?status={status}&url={url}"

	ctx := context.Background()
	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {})

	handler, err := redirecterrors.New(ctx, next, cfg, "redirecterrors-plugin")
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost", nil)
	if err != nil {
		t.Fatal(err)
	}

	handler.ServeHTTP(recorder, req)

	resp := recorder.Result()
	assertCode(t, resp, 200)
	assertHeader(t, resp, "Location", "")
}

func assertCode(t *testing.T, resp *http.Response, expected int) {
	t.Helper()

	if resp.StatusCode != expected {
		t.Errorf("invalid status value: %d", resp.StatusCode)
	}
}

func assert(t *testing.T, condition bool) bool {
	t.Helper()

	if !condition {
		t.Error("Assertation failed")
	}
	return condition
}

func assertHeader(t *testing.T, resp *http.Response, key, expected string) {
	t.Helper()

	if resp.Header.Get(key) != expected {
		t.Errorf("invalid header value: %s", resp.Header.Get(key))
	}
}

func TestRedirectWithCustomHeaders(t *testing.T) {
	cfg := redirecterrors.CreateConfig()
	cfg.Status = []string{"401"}
	cfg.Target = "http://target/?status={status}&url={url}"
	cfg.OutputStatus = 302
	cfg.OutputAddHeaders = map[string]string{
		"Set-Cookie":      "session=; Path=/; Domain=.example.com; HttpOnly; Secure; Max-Age=0",
		"X-Custom-Header": "custom-value",
	}

	ctx := context.Background()
	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) { rw.WriteHeader(401) })

	handler, err := redirecterrors.New(ctx, next, cfg, "redirecterrors-plugin")
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost", nil)
	if err != nil {
		t.Fatal(err)
	}

	handler.ServeHTTP(recorder, req)

	resp := recorder.Result()
	assertHeader(t, resp, "Location", "http://target/?status=401&url=http%3A%2F%2Flocalhost")
	assertHeader(t, resp, "Set-Cookie", "session=; Path=/; Domain=.example.com; HttpOnly; Secure; Max-Age=0")
	assertHeader(t, resp, "X-Custom-Header", "custom-value")
	assertCode(t, resp, 302)
}

func assertNoHeader(t *testing.T, resp *http.Response, key string) {
	t.Helper()

	if resp.Header.Get(key) != "" {
		t.Errorf("header %s should not be set, but got: %s", key, resp.Header.Get(key))
	}
}

func TestRedirectWithRemoveHeaders(t *testing.T) {
	cfg := redirecterrors.CreateConfig()
	cfg.Status = []string{"401"}
	cfg.Target = "http://target/?status={status}&url={url}"
	cfg.OutputStatus = 302
	cfg.OutputRemoveHeaders = []string{
		"^Authentik-Proxy-.+$",
		"^[^-]+-Tk-.+$",
	}

	ctx := context.Background()
	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		// Simulate forwardAuth adding headers (Go canonicalizes header names)
		rw.Header().Set("Authentik-Proxy-User", "testuser")
		rw.Header().Set("Authentik-Proxy-Groups", "admins")
		rw.Header().Set("App-Tk-Session", "abc123")
		rw.Header().Set("X-Should-Remain", "keep-this")
		rw.WriteHeader(401)
	})

	handler, err := redirecterrors.New(ctx, next, cfg, "redirecterrors-plugin")
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost", nil)
	if err != nil {
		t.Fatal(err)
	}

	handler.ServeHTTP(recorder, req)

	resp := recorder.Result()
	assertHeader(t, resp, "Location", "http://target/?status=401&url=http%3A%2F%2Flocalhost")
	assertCode(t, resp, 302)

	// These headers should be removed
	assertNoHeader(t, resp, "Authentik-Proxy-User")
	assertNoHeader(t, resp, "Authentik-Proxy-Groups")
	assertNoHeader(t, resp, "App-Tk-Session")

	// This header should remain
	assertHeader(t, resp, "X-Should-Remain", "keep-this")
}

func TestInvalidRegexPattern(t *testing.T) {
	cfg := redirecterrors.CreateConfig()
	cfg.Status = []string{"401"}
	cfg.Target = "http://target/"
	cfg.OutputRemoveHeaders = []string{
		"[invalid(", // Invalid regex
	}

	ctx := context.Background()
	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(401)
	})

	_, err := redirecterrors.New(ctx, next, cfg, "redirecterrors-plugin")
	if err == nil {
		t.Fatal("expected error for invalid regex, got nil")
	}
}

func TestRedirectWithAddCookies(t *testing.T) {
	cfg := redirecterrors.CreateConfig()
	cfg.Status = []string{"401"}
	cfg.Target = "http://target/?status={status}&url={url}"
	cfg.OutputStatus = 302
	cfg.OutputAddCookies = []string{
		"session=123; Path=/; Domain=.example.com; HttpOnly; Secure",
		"mycookie=yes; Path=/; Domain=.example.com; HttpOnly; Secure",
	}

	ctx := context.Background()
	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(401)
	})

	handler, err := redirecterrors.New(ctx, next, cfg, "redirecterrors-plugin")
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost", nil)
	if err != nil {
		t.Fatal(err)
	}

	handler.ServeHTTP(recorder, req)

	resp := recorder.Result()
	assertCode(t, resp, 302)

	// Check that cookies were set
	cookies := resp.Cookies()
	if len(cookies) != 2 {
		t.Errorf("expected 2 cookies, got %d", len(cookies))
	}

	// Find session cookie
	sessionFound := false
	mycookieFound := false
	for _, cookie := range cookies {
		if cookie.Name == "session" && cookie.Value == "123" {
			sessionFound = true
		}
		if cookie.Name == "mycookie" && cookie.Value == "yes" {
			mycookieFound = true
		}
	}

	if !sessionFound {
		t.Error("session cookie not found or has wrong value")
	}
	if !mycookieFound {
		t.Error("mycookie not found or has wrong value")
	}
}

func TestRedirectWithRemoveCookies(t *testing.T) {
	cfg := redirecterrors.CreateConfig()
	cfg.Status = []string{"401"}
	cfg.Target = "http://target/?status={status}&url={url}"
	cfg.OutputStatus = 302
	cfg.OutputRemoveCookies = []string{
		"^authentik_proxy_.+$",
		"^[^_]+_tk_.+$",
	}

	ctx := context.Background()
	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(401)
	})

	handler, err := redirecterrors.New(ctx, next, cfg, "redirecterrors-plugin")
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Add cookies to the request that should be removed
	req.AddCookie(&http.Cookie{Name: "authentik_proxy_user", Value: "testuser"})
	req.AddCookie(&http.Cookie{Name: "authentik_proxy_groups", Value: "admins"})
	req.AddCookie(&http.Cookie{Name: "app_tk_session", Value: "abc123"})
	req.AddCookie(&http.Cookie{Name: "keep_this", Value: "keep"})

	handler.ServeHTTP(recorder, req)

	resp := recorder.Result()
	assertCode(t, resp, 302)

	// Check that deletion cookies were set for matching cookies
	cookies := resp.Cookies()

	// Should have deletion cookies for authentik_proxy_*, app_*_tk_*
	deletionCookies := make(map[string]bool)
	for _, cookie := range cookies {
		if cookie.Value == "" && cookie.MaxAge < 0 {
			deletionCookies[cookie.Name] = true
		}
	}

	// Check that matching cookies were marked for deletion
	if !deletionCookies["authentik_proxy_user"] {
		t.Error("authentik_proxy_user should have deletion cookie")
	}
	if !deletionCookies["authentik_proxy_groups"] {
		t.Error("authentik_proxy_groups should have deletion cookie")
	}
	if !deletionCookies["app_tk_session"] {
		t.Error("app_tk_session should have deletion cookie")
	}

	// keep_this should not have a deletion cookie
	if deletionCookies["keep_this"] {
		t.Error("keep_this should not have deletion cookie")
	}
}

func TestInvalidCookieRegexPattern(t *testing.T) {
	cfg := redirecterrors.CreateConfig()
	cfg.Status = []string{"401"}
	cfg.Target = "http://target/"
	cfg.OutputRemoveCookies = []string{
		"[invalid(", // Invalid regex
	}

	ctx := context.Background()
	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(401)
	})

	_, err := redirecterrors.New(ctx, next, cfg, "redirecterrors-plugin")
	if err == nil {
		t.Fatal("expected error for invalid cookie regex, got nil")
	}
}

func TestRedirectWithBody(t *testing.T) {
	// Test codeCatcher.Write() path
	cfg := redirecterrors.CreateConfig()
	cfg.Status = []string{"401"}
	cfg.Target = "http://target/?status={status}&url={url}"
	cfg.OutputStatus = 302

	ctx := context.Background()
	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(401)
		_, _ = rw.Write([]byte("error body")) // This triggers Write path
		_, _ = rw.Write([]byte("more")) // Ignore error for test
	})

	handler, err := redirecterrors.New(ctx, next, cfg, "redirecterrors-plugin")
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost", nil)
	if err != nil {
		t.Fatal(err)
	}

	handler.ServeHTTP(recorder, req)

	resp := recorder.Result()
	assertCode(t, resp, 302)
	assertHeader(t, resp, "Location", "http://target/?status=401&url=http%3A%2F%2Flocalhost")
	// Body should be "Redirecting" not the original "error body"
	body := recorder.Body.String()
	if body != "Redirecting" {
		t.Errorf("expected 'Redirecting', got '%s'", body)
	}
}

func TestRedirectWithoutWriteHeader(t *testing.T) {
	// Test when backend doesn't call WriteHeader (defaults to 200)
	cfg := redirecterrors.CreateConfig()
	cfg.Status = []string{"200-299"}
	cfg.Target = "http://target/?status={status}&url={url}"
	cfg.OutputStatus = 302

	ctx := context.Background()
	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		// Don't call WriteHeader, just write body - defaults to 200
		_, _ = rw.Write([]byte("success"))
	})

	handler, err := redirecterrors.New(ctx, next, cfg, "redirecterrors-plugin")
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost", nil)
	if err != nil {
		t.Fatal(err)
	}

	handler.ServeHTTP(recorder, req)

	resp := recorder.Result()
	assertCode(t, resp, 302)
	assertHeader(t, resp, "Location", "http://target/?status=200&url=http%3A%2F%2Flocalhost")
}

func TestStatusCodeRanges(t *testing.T) {
	// Test HTTPCodeRanges.Contains() directly
	cfg := redirecterrors.CreateConfig()
	cfg.Status = []string{"200-202", "404", "500-503"}
	cfg.Target = "http://target/"

	ctx := context.Background()

	// Test various status codes
	testCases := []struct {
		statusCode int
		shouldRedirect bool
	}{
		{200, true},   // in range 200-202
		{201, true},   // in range 200-202
		{202, true},   // in range 200-202
		{203, false},  // not in any range
		{404, true},   // exact match
		{500, true},   // in range 500-503
		{503, true},   // in range 500-503
		{504, false},  // not in any range
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("status_%d", tc.statusCode), func(t *testing.T) {
			recorder := httptest.NewRecorder()
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost", nil)

			next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
				rw.WriteHeader(tc.statusCode)
			})

			handler, err := redirecterrors.New(ctx, next, cfg, "redirecterrors-plugin")
			if err != nil {
				t.Fatal(err)
			}

			handler.ServeHTTP(recorder, req)

			resp := recorder.Result()
			if tc.shouldRedirect {
				if resp.StatusCode != 302 {
					t.Errorf("expected redirect (302) for status %d, got %d", tc.statusCode, resp.StatusCode)
				}
			} else {
				if resp.StatusCode != tc.statusCode {
					t.Errorf("expected passthrough with status %d, got %d", tc.statusCode, resp.StatusCode)
				}
			}
		})
	}
}

func TestSingleStatusCodeWithoutDash(t *testing.T) {
	// Test that a single status code without dash works
	cfg := redirecterrors.CreateConfig()
	cfg.Status = []string{"404"} // Single code, no dash
	cfg.Target = "http://target/"

	ctx := context.Background()
	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(404)
	})

	handler, err := redirecterrors.New(ctx, next, cfg, "redirecterrors-plugin")
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost", nil)
	if err != nil {
		t.Fatal(err)
	}

	handler.ServeHTTP(recorder, req)

	resp := recorder.Result()
	assertCode(t, resp, 302)
	// Target doesn't have template variables, so it's just the target
	location := resp.Header.Get("Location")
	if location != "http://target/" {
		t.Errorf("expected location 'http://target/', got '%s'", location)
	}
}

func TestInvalidStatusCodeRange(t *testing.T) {
	// Test invalid status code range
	cfg := redirecterrors.CreateConfig()
	cfg.Status = []string{"invalid"}
	cfg.Target = "http://target/"

	ctx := context.Background()
	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {})

	_, err := redirecterrors.New(ctx, next, cfg, "redirecterrors-plugin")
	if err == nil {
		t.Fatal("expected error for invalid status code, got nil")
	}
}

func TestFullURLWithForwardedHeaders(t *testing.T) {
	// Test URL reconstruction with X-Forwarded headers
	cfg := redirecterrors.CreateConfig()
	cfg.Status = []string{"401"}
	cfg.Target = "http://target/?return={url}"
	cfg.OutputStatus = 302

	ctx := context.Background()
	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(401)
	})

	handler, err := redirecterrors.New(ctx, next, cfg, "redirecterrors-plugin")
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost/api/test", nil)
	if err != nil {
		t.Fatal(err)
	}
	// Set X-Forwarded headers like Traefik does
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "example.com")

	handler.ServeHTTP(recorder, req)

	resp := recorder.Result()
	location := resp.Header.Get("Location")
	// The URL is URL-encoded in the template replacement
	expected := "http://target/?return=https%3A%2F%2Fexample.com%2Fapi%2Ftest"
	if location != expected {
		t.Errorf("expected location '%s', got '%s'", expected, location)
	}
}

func TestEmptyAddHeaders(t *testing.T) {
	// Test with empty outputAddHeaders map (default)
	cfg := redirecterrors.CreateConfig()
	cfg.Status = []string{"401"}
	cfg.Target = "http://target/"
	cfg.OutputAddHeaders = map[string]string{} // Empty map

	ctx := context.Background()
	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(401)
	})

	handler, err := redirecterrors.New(ctx, next, cfg, "redirecterrors-plugin")
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost", nil)
	if err != nil {
		t.Fatal(err)
	}

	handler.ServeHTTP(recorder, req)

	resp := recorder.Result()
	assertCode(t, resp, 302)
	// Should not crash with empty headers map
}

func TestEmptyRemoveLists(t *testing.T) {
	// Test with empty outputRemoveHeaders and outputRemoveCookies (default)
	cfg := redirecterrors.CreateConfig()
	cfg.Status = []string{"401"}
	cfg.Target = "http://target/"
	cfg.OutputRemoveHeaders = []string{}
	cfg.OutputRemoveCookies = []string{}

	ctx := context.Background()
	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(401)
	})

	handler, err := redirecterrors.New(ctx, next, cfg, "redirecterrors-plugin")
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost", nil)
	if err != nil {
		t.Fatal(err)
	}

	handler.ServeHTTP(recorder, req)

	resp := recorder.Result()
	assertCode(t, resp, 302)
}

func TestAddAndRemoveCookiesTogether(t *testing.T) {
	// Test using both outputAddCookies and outputRemoveCookies together
	cfg := redirecterrors.CreateConfig()
	cfg.Status = []string{"401"}
	cfg.Target = "http://target/"
	cfg.OutputAddCookies = []string{
		"new_cookie=value; Path=/; HttpOnly; Secure",
	}
	cfg.OutputRemoveCookies = []string{
		"^old_cookie$",
	}

	ctx := context.Background()
	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(401)
	})

	handler, err := redirecterrors.New(ctx, next, cfg, "redirecterrors-plugin")
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(&http.Cookie{Name: "old_cookie", Value: "delete_me"})
	req.AddCookie(&http.Cookie{Name: "keep_cookie", Value: "keep_me"})

	handler.ServeHTTP(recorder, req)

	resp := recorder.Result()
	cookies := resp.Cookies()

	// Should have new_cookie added
	newCookieFound := false
	oldCookieDeleted := false
	keepCookieNotDeleted := true
	for _, cookie := range cookies {
		if cookie.Name == "new_cookie" && cookie.Value == "value" {
			newCookieFound = true
		}
		if cookie.Name == "old_cookie" && cookie.Value == "" {
			oldCookieDeleted = true
		}
		if cookie.Name == "keep_cookie" && cookie.Value == "" {
			keepCookieNotDeleted = false
		}
	}

	if !newCookieFound {
		t.Error("new_cookie should be added")
	}
	if !oldCookieDeleted {
		t.Error("old_cookie should be deleted")
	}
	if !keepCookieNotDeleted {
		t.Error("keep_cookie should not be deleted")
	}
}

func TestCookieNameExtraction(t *testing.T) {
	// Indirectly test extractCookieName via outputAddCookies
	cfg := redirecterrors.CreateConfig()
	cfg.Status = []string{"401"}
	cfg.Target = "http://target/"
	cfg.OutputAddCookies = []string{
		"simple=value",
		"complex=value; Path=/; Domain=.example.com; HttpOnly; Secure; SameSite=Lax",
	}

	ctx := context.Background()
	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(401)
	})

	handler, err := redirecterrors.New(ctx, next, cfg, "redirecterrors-plugin")
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost", nil)
	if err != nil {
		t.Fatal(err)
	}

	handler.ServeHTTP(recorder, req)

	resp := recorder.Result()
	cookies := resp.Cookies()

	if len(cookies) != 2 {
		t.Errorf("expected 2 cookies, got %d", len(cookies))
	}

	// Check cookie names
	expectedNames := []string{"simple", "complex"}
	for _, expectedName := range expectedNames {
		found := false
		for _, cookie := range cookies {
			if cookie.Name == expectedName {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("cookie '%s' not found", expectedName)
		}
	}
}

func TestRemoveCookiesNoMatch(t *testing.T) {
	// Test outputRemoveCookies with no matching cookies
	cfg := redirecterrors.CreateConfig()
	cfg.Status = []string{"401"}
	cfg.Target = "http://target/"
	cfg.OutputRemoveCookies = []string{
		"^nomatch_.*$",
	}

	ctx := context.Background()
	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(401)
	})

	handler, err := redirecterrors.New(ctx, next, cfg, "redirecterrors-plugin")
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(&http.Cookie{Name: "keep_me", Value: "value"})

	handler.ServeHTTP(recorder, req)

	resp := recorder.Result()
	cookies := resp.Cookies()

	// Should have no deletion cookies
	for _, cookie := range cookies {
		if cookie.Value == "" {
			t.Errorf("unexpected deletion cookie: %s", cookie.Name)
		}
	}
}

func TestOutputStatusCodes(t *testing.T) {
	// Test different output status codes
	testCases := []struct {
		outputStatus int
		expected     int
	}{
		{301, 301},
		{302, 302},
		{303, 303},
		{307, 307},
		{308, 308},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("output_%d", tc.outputStatus), func(t *testing.T) {
			cfg := redirecterrors.CreateConfig()
			cfg.Status = []string{"401"}
			cfg.Target = "http://target/"
			cfg.OutputStatus = tc.outputStatus

			ctx := context.Background()
			next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
				rw.WriteHeader(401)
			})

			handler, err := redirecterrors.New(ctx, next, cfg, "redirecterrors-plugin")
			if err != nil {
				t.Fatal(err)
			}

			recorder := httptest.NewRecorder()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost", nil)
			if err != nil {
				t.Fatal(err)
			}

			handler.ServeHTTP(recorder, req)

			resp := recorder.Result()
			if resp.StatusCode != tc.expected {
				t.Errorf("expected status %d, got %d", tc.expected, resp.StatusCode)
			}
		})
	}
}

func TestRemoveHeadersMultiplePatterns(t *testing.T) {
	// Test that a header matching multiple patterns is only removed once
	cfg := redirecterrors.CreateConfig()
	cfg.Status = []string{"401"}
	cfg.Target = "http://target/"
	cfg.OutputRemoveHeaders = []string{
		"^X-.*$",
		"^X-A.*$", // This also matches X-Auth-Test
	}

	ctx := context.Background()
	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.Header().Set("X-Auth-Test", "value")
		rw.Header().Set("X-Other", "value2")
		rw.Header().Set("Keep-This", "value3")
		rw.WriteHeader(401)
	})

	handler, err := redirecterrors.New(ctx, next, cfg, "redirecterrors-plugin")
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost", nil)
	if err != nil {
		t.Fatal(err)
	}

	handler.ServeHTTP(recorder, req)

	resp := recorder.Result()
	// X-Auth-Test and X-Other should be removed, Keep-This should remain
	if resp.Header.Get("X-Auth-Test") != "" {
		t.Error("X-Auth-Test should be removed")
	}
	if resp.Header.Get("X-Other") != "" {
		t.Error("X-Other should be removed")
	}
	if resp.Header.Get("Keep-This") != "value3" {
		t.Error("Keep-This should remain")
	}
}

func TestHTTPCodeRangesContains(t *testing.T) {
	// Test HTTPCodeRanges.Contains() method directly
	ranges, err := redirecterrors.NewHTTPCodeRanges([]string{"200-202", "404", "500-503"})
	if err != nil {
		t.Fatal(err)
	}

	testCases := []struct {
		code   int
		match  bool
	}{
		{199, false},
		{200, true},
		{201, true},
		{202, true},
		{203, false},
		{404, true},
		{405, false},
		{499, false},
		{500, true},
		{503, true},
		{504, false},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("code_%d", tc.code), func(t *testing.T) {
			result := ranges.Contains(tc.code)
			if result != tc.match {
				t.Errorf("Contains(%d) = %v, want %v", tc.code, result, tc.match)
			}
		})
	}
}

func TestNewHTTPCodeRangesSingleCode(t *testing.T) {
	// Test single code without dash gets converted to range
	ranges, err := redirecterrors.NewHTTPCodeRanges([]string{"404"})
	if err != nil {
		t.Fatal(err)
	}

	if !ranges.Contains(404) {
		t.Error("single code 404 should match")
	}
	if ranges.Contains(403) {
		t.Error("403 should not match range for 404")
	}
	if ranges.Contains(405) {
		t.Error("405 should not match range for 404")
	}
}

func TestRedirectWithMultipleCookiesSameName(t *testing.T) {
	// Test adding multiple cookies with same name (last one wins via Set, but Add appends)
	cfg := redirecterrors.CreateConfig()
	cfg.Status = []string{"401"}
	cfg.Target = "http://target/"
	cfg.OutputAddCookies = []string{
		"session=first; Path=/",
		"session=second; Path=/",
	}

	ctx := context.Background()
	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(401)
	})

	handler, err := redirecterrors.New(ctx, next, cfg, "redirecterrors-plugin")
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost", nil)
	if err != nil {
		t.Fatal(err)
	}

	handler.ServeHTTP(recorder, req)

	resp := recorder.Result()
	// Check that multiple cookies with same name are both added
	// The http.Response.Cookies() parsing deduplicates, so check header values directly
	setCookieHeaders := resp.Header.Values("Set-Cookie")
	if len(setCookieHeaders) != 2 {
		t.Errorf("expected 2 Set-Cookie headers, got %d: %v", len(setCookieHeaders), setCookieHeaders)
	}
}

func TestRemoveCookiesDuplicatePatterns(t *testing.T) {
	// Test that a cookie matching duplicate patterns is only removed once
	cfg := redirecterrors.CreateConfig()
	cfg.Status = []string{"401"}
	cfg.Target = "http://target/"
	cfg.OutputRemoveCookies = []string{
		"^test_.*$",
		"^test_cookie$",  // More specific pattern also matches
	}

	ctx := context.Background()
	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(401)
	})

	handler, err := redirecterrors.New(ctx, next, cfg, "redirecterrors-plugin")
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(&http.Cookie{Name: "test_cookie", Value: "value"})

	handler.ServeHTTP(recorder, req)

	resp := recorder.Result()
	// Count deletion cookies for test_cookie
	deletionCount := 0
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "test_cookie" && cookie.Value == "" {
			deletionCount++
		}
	}
	// Should only have ONE deletion cookie, not two
	if deletionCount != 1 {
		t.Errorf("expected 1 deletion cookie, got %d", deletionCount)
	}
}

func TestStatusCodeWithNoMatch(t *testing.T) {
	// Test status code that doesn't match any range passes through
	cfg := redirecterrors.CreateConfig()
	cfg.Status = []string{"500-599"}
	cfg.Target = "http://target/"

	ctx := context.Background()
	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(200)
		_, _ = rw.Write([]byte("OK"))
	})

	handler, err := redirecterrors.New(ctx, next, cfg, "redirecterrors-plugin")
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost", nil)
	if err != nil {
		t.Fatal(err)
	}

	handler.ServeHTTP(recorder, req)

	resp := recorder.Result()
	assertCode(t, resp, 200) // Should passthrough
	if resp.Header.Get("Location") != "" {
		t.Error("should not set Location header for passthrough")
	}
	body := recorder.Body.String()
	if body != "OK" {
		t.Errorf("expected body 'OK', got '%s'", body)
	}
}

func TestNonFilteredCodeHeadersCopied(t *testing.T) {
	// Test that headers are copied when code is not filtered (passthrough)
	cfg := redirecterrors.CreateConfig()
	cfg.Status = []string{"500-599"}
	cfg.Target = "http://target/"

	ctx := context.Background()
	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.Header().Set("X-Custom", "value")
		rw.Header().Set("X-Another", "value2")
		rw.WriteHeader(200)
	})

	handler, err := redirecterrors.New(ctx, next, cfg, "redirecterrors-plugin")
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost", nil)
	if err != nil {
		t.Fatal(err)
	}

	handler.ServeHTTP(recorder, req)

	resp := recorder.Result()
	assertCode(t, resp, 200)
	// Headers should be copied for passthrough
	if resp.Header.Get("X-Custom") != "value" {
		t.Error("X-Custom header should be present")
	}
	if resp.Header.Get("X-Another") != "value2" {
		t.Error("X-Another header should be present")
	}
}

func TestAddCookiesWithEmptyString(t *testing.T) {
	// Test edge case: cookie string with just whitespace
	// The empty cookie name is added as a Set-Cookie header but Go's Cookies() skips it
	cfg := redirecterrors.CreateConfig()
	cfg.Status = []string{"401"}
	cfg.Target = "http://target/"
	cfg.OutputAddCookies = []string{
		"  ",  // whitespace only - results in empty name
		"valid=value",
	}

	ctx := context.Background()
	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(401)
	})

	handler, err := redirecterrors.New(ctx, next, cfg, "redirecterrors-plugin")
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost", nil)
	if err != nil {
		t.Fatal(err)
	}

	handler.ServeHTTP(recorder, req)

	resp := recorder.Result()
	// Check Set-Cookie headers directly since empty-named cookies are skipped by Cookies()
	setCookieHeaders := resp.Header.Values("Set-Cookie")
	// Should have 2 Set-Cookie headers (one with empty name, one valid)
	if len(setCookieHeaders) != 2 {
		t.Logf("Got Set-Cookie headers: %v", setCookieHeaders)
		t.Errorf("expected 2 Set-Cookie headers, got %d", len(setCookieHeaders))
	}
	// But Cookies() should only return 1 (skips empty name)
	cookies := resp.Cookies()
	if len(cookies) != 1 {
		t.Errorf("expected 1 parsed cookie (empty names skipped), got %d", len(cookies))
	}
}

func TestRemoveCookiesWithEmptyCookieName(t *testing.T) {
	// Test edge case: request with cookie that has empty name after extraction
	cfg := redirecterrors.CreateConfig()
	cfg.Status = []string{"401"}
	cfg.Target = "http://target/"
	cfg.OutputRemoveCookies = []string{
		"^.*$",  // Match everything
	}

	ctx := context.Background()
	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(401)
	})

	handler, err := redirecterrors.New(ctx, next, cfg, "redirecterrors-plugin")
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(&http.Cookie{Name: "test", Value: "value"})

	handler.ServeHTTP(recorder, req)

	resp := recorder.Result()
	// Should have a deletion cookie for "test"
	cookies := resp.Cookies()
	found := false
	for _, cookie := range cookies {
		if cookie.Name == "test" && cookie.Value == "" {
			found = true
		}
	}
	if !found {
		t.Error("expected deletion cookie for 'test'")
	}
}

func TestAddHeadersOverrideUpstream(t *testing.T) {
	// Test that outputAddHeaders can override upstream headers
	cfg := redirecterrors.CreateConfig()
	cfg.Status = []string{"401"}
	cfg.Target = "http://target/"
	cfg.OutputAddHeaders = map[string]string{
		"X-Override": "new-value",
	}

	ctx := context.Background()
	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.Header().Set("X-Override", "old-value")
		rw.WriteHeader(401)
	})

	handler, err := redirecterrors.New(ctx, next, cfg, "redirecterrors-plugin")
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost", nil)
	if err != nil {
		t.Fatal(err)
	}

	handler.ServeHTTP(recorder, req)

	resp := recorder.Result()
	// outputAddHeaders should override upstream header
	if resp.Header.Get("X-Override") != "new-value" {
		t.Errorf("expected 'new-value', got '%s'", resp.Header.Get("X-Override"))
	}
}

func TestLocationHeaderOverridden(t *testing.T) {
	// Test that Location header is always set to redirect target (even if upstream set it)
	cfg := redirecterrors.CreateConfig()
	cfg.Status = []string{"401"}
	cfg.Target = "http://redirect-target.com"

	ctx := context.Background()
	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.Header().Set("Location", "http://wrong-location.com")
		rw.WriteHeader(401)
	})

	handler, err := redirecterrors.New(ctx, next, cfg, "redirecterrors-plugin")
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost", nil)
	if err != nil {
		t.Fatal(err)
	}

	handler.ServeHTTP(recorder, req)

	resp := recorder.Result()
	location := resp.Header.Get("Location")
	if location != "http://redirect-target.com" {
		t.Errorf("expected 'http://redirect-target.com', got '%s'", location)
	}
}

func TestStatusCodeEdgeCases(t *testing.T) {
	// Test edge cases for status code ranges
	testCases := []struct {
		name   string
		status []string
		code   int
		match  bool
	}{
		{"single code", []string{"404"}, 404, true},
		{"single code non-match", []string{"404"}, 405, false},
		{"range boundaries", []string{"400-403"}, 400, true},
		{"range boundaries", []string{"400-403"}, 403, true},
		{"range boundaries", []string{"400-403"}, 404, false},
		{"multiple ranges", []string{"200-299", "400-404"}, 299, true},
		{"multiple ranges", []string{"200-299", "400-404"}, 300, false},
		{"multiple ranges", []string{"200-299", "400-404"}, 404, true},
		{"multiple ranges", []string{"200-299", "400-404"}, 405, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := redirecterrors.CreateConfig()
			cfg.Status = tc.status
			cfg.Target = "http://target/"

			ctx := context.Background()
			next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
				rw.WriteHeader(tc.code)
			})

			handler, err := redirecterrors.New(ctx, next, cfg, "redirecterrors-plugin")
			if err != nil {
				t.Fatal(err)
			}

			recorder := httptest.NewRecorder()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost", nil)
			if err != nil {
				t.Fatal(err)
			}

			handler.ServeHTTP(recorder, req)

			resp := recorder.Result()
			if tc.match {
				if resp.StatusCode != 302 {
					t.Errorf("expected redirect for status %d with range %v", tc.code, tc.status)
				}
			} else {
				if resp.StatusCode != tc.code {
					t.Errorf("expected passthrough for status %d with range %v", tc.code, tc.status)
				}
			}
		})
	}
}

func TestRemoveHeadersNonMatching(t *testing.T) {
	// Test that headers not matching patterns are kept
	cfg := redirecterrors.CreateConfig()
	cfg.Status = []string{"401"}
	cfg.Target = "http://target/"
	cfg.OutputRemoveHeaders = []string{
		"^X-Private-.*$",
	}

	ctx := context.Background()
	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.Header().Set("X-Public-Data", "public")
		rw.Header().Set("X-Private-Key", "secret")
		rw.Header().Set("X-Other", "value")
		rw.WriteHeader(401)
	})

	handler, err := redirecterrors.New(ctx, next, cfg, "redirecterrors-plugin")
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost", nil)
	if err != nil {
		t.Fatal(err)
	}

	handler.ServeHTTP(recorder, req)

	resp := recorder.Result()
	// X-Public-Data and X-Other should remain
	if resp.Header.Get("X-Public-Data") != "public" {
		t.Error("X-Public-Data should remain")
	}
	if resp.Header.Get("X-Other") != "value" {
		t.Error("X-Other should remain")
	}
	// X-Private-Key should be removed
	if resp.Header.Get("X-Private-Key") != "" {
		t.Error("X-Private-Key should be removed")
	}
}

func TestAllEmptyConfig(t *testing.T) {
	// Test with all optional config empty (defaults)
	cfg := redirecterrors.CreateConfig()
	cfg.Status = []string{} // Empty - matches nothing
	cfg.Target = "http://target/"
	// OutputStatus defaults to 302
	// All other fields default to empty/nil

	ctx := context.Background()
	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(200)
		_, _ = rw.Write([]byte("OK"))
	})

	handler, err := redirecterrors.New(ctx, next, cfg, "redirecterrors-plugin")
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost", nil)
	if err != nil {
		t.Fatal(err)
	}

	handler.ServeHTTP(recorder, req)

	resp := recorder.Result()
	// Empty status list means no redirect, should passthrough
	assertCode(t, resp, 200)
}

func TestRedirectWithMultipleStatusCodes(t *testing.T) {
	// Test with multiple status codes in config
	cfg := redirecterrors.CreateConfig()
	cfg.Status = []string{"401", "403", "500-503"}
	cfg.Target = "http://target/?error={status}"
	cfg.OutputStatus = 307

	ctx := context.Background()

	testCases := []int{401, 403, 500, 501, 502, 503}
	for _, statusCode := range testCases {
		t.Run(fmt.Sprintf("status_%d", statusCode), func(t *testing.T) {
			next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
				rw.WriteHeader(statusCode)
			})

			handler, err := redirecterrors.New(ctx, next, cfg, "redirecterrors-plugin")
			if err != nil {
				t.Fatal(err)
			}

			recorder := httptest.NewRecorder()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost", nil)
			if err != nil {
				t.Fatal(err)
			}

			handler.ServeHTTP(recorder, req)

			resp := recorder.Result()
			assertCode(t, resp, 307)
			expectedLocation := fmt.Sprintf("http://target/?error=%d", statusCode)
			if resp.Header.Get("Location") != expectedLocation {
				t.Errorf("expected location '%s', got '%s'", expectedLocation, resp.Header.Get("Location"))
			}
		})
	}
}
