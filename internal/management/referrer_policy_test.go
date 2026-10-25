package management

import (
    "net/http"
    "net/http/httptest"
    "net/url"
    "path/filepath"
    "strings"
    "testing"
)

// Regression: an actual Chrome HTML form submission from the public HTTPS
// settings page failed CSRF validation when Referrer-Policy was no-referrer.
// The response must permit a normal same-origin POST's Origin header while
// retaining the existing CSRF token and strict origin validation.
func TestPublicSettingsFormReferrerPolicyAndCSRF(t *testing.T) {
    handler, _, project, guard := setup(t)
    const publicOrigin = "https://docklantern.example.com"
    if err := guard.SetPublicOrigin(publicOrigin); err != nil {
        t.Fatal(err)
    }
    makeRequest := func(method, path, origin, fetchSite, body string) *httptest.ResponseRecorder {
        t.Helper()
        req := httptest.NewRequest(method, publicOrigin+path, strings.NewReader(body))
        req.Host = "docklantern.example.com"
        req.Header.Set("X-Forwarded-Proto", "https")
        req.SetBasicAuth("admin", strings.Repeat("very-secure-demo-password-", 2))
        if method == http.MethodPost {
            req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
        }
        if origin != "" { req.Header.Set("Origin", origin) }
        if fetchSite != "" { req.Header.Set("Sec-Fetch-Site", fetchSite) }
        rec := httptest.NewRecorder()
        handler.ServeHTTP(rec, req)
        return rec
    }

    page := makeRequest(http.MethodGet, "/settings", "", "same-origin", "")
    if page.Code != http.StatusOK { t.Fatalf("GET settings returned %d", page.Code) }
    if got := page.Header().Get("Referrer-Policy"); got != "same-origin" {
        t.Fatalf("Referrer-Policy=%q, want same-origin", got)
    }
    if !strings.Contains(page.Body.String(), "name=\"csrf_token\" value=\""+guard.Token()+"\"") {
        t.Fatal("settings form missing current CSRF token")
    }

    form := url.Values{
        "csrf_token": {guard.Token()},
        "name": {"example"},
        "project_dir": {project},
        "files": {filepath.Join(project, "compose.yaml")},
        "services": {"web"},
    }.Encode()
    valid := makeRequest(http.MethodPost, "/settings/projects", publicOrigin, "same-origin", form)
    if valid.Code != http.StatusSeeOther {
        t.Fatalf("valid same-origin registration returned %d, want 303", valid.Code)
    }
    for _, origin := range []string{"null", "https://attacker.example.com"} {
        rejected := makeRequest(http.MethodPost, "/settings/projects", origin, "same-origin", form)
        if rejected.Code != http.StatusForbidden {
            t.Fatalf("origin %q returned %d, want 403", origin, rejected.Code)
        }
    }
}
