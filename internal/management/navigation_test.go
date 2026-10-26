package management

import (
    "net/http"
    "regexp"
    "strings"
    "testing"
)

// Every HTML route must render the same navigation shell. This also catches
// the earlier bug where Settings and Events were separate pages with no sidebar.
func TestSharedNavigationOnAllHTMLPages(t *testing.T) {
    handler, _, _, _ := setup(t)
    activeLink := regexp.MustCompile(`<a class="nav-link active" aria-current="page" href="([^"]+)"`)
    var commonSidebar string
    for _, tc := range []struct {
        path string
        wantActive string
        pageText string
    }{
        {"/", "/", "Image Overview"},
        {"/images", "/images", "Watched images"},
        {"/services", "/services", "Compose services"},
        {"/settings", "/settings", "Monitoring settings"},
        {"/events", "/events", "Events"},
    } {
        t.Run(tc.path, func(t *testing.T) {
            response := request(handler, http.MethodGet, tc.path, "", true)
            if response.Code != http.StatusOK {
                t.Fatalf("GET %s returned %d: %s", tc.path, response.Code, response.Body.String())
            }
            page := response.Body.String()
            start := strings.Index(page, `<aside class="sidebar"`)
            end := strings.Index(page, "</aside>")
            if start < 0 || end <= start {
                t.Fatalf("%s has no sidebar", tc.path)
            }
            sidebar := page[start : end+len("</aside>")]
            if commonSidebar == "" {
                // Ignore the one expected difference: active page indication.
                commonSidebar = activeLink.ReplaceAllString(sidebar, `<a class="nav-link" href="$1"`)
            } else {
                // All pages must use exactly the same navigation structure.
                normalized := activeLink.ReplaceAllString(sidebar, `<a class="nav-link" href="$1"`)
                // Strip active class from other links before comparing.
                normalized = strings.ReplaceAll(normalized, ` class="nav-link active"`, ` class="nav-link"`)
                normalizedCommon := strings.ReplaceAll(commonSidebar, ` class="nav-link active"`, ` class="nav-link"`)
                if normalized != normalizedCommon {
                    t.Fatalf("%s sidebar differs from shared navigation", tc.path)
                }
            }
            if strings.Count(sidebar, `aria-current="page"`) != 1 {
                t.Errorf("%s must have exactly one active navigation link", tc.path)
            }
            active := activeLink.FindStringSubmatch(sidebar)
            if len(active) != 2 || active[1] != tc.wantActive {
                t.Errorf("%s active link = %v; want %q", tc.path, active, tc.wantActive)
            }
            for _, link := range []string{"/", "/images", "/services", "/events", "/settings"} {
                if !strings.Contains(sidebar, `href="`+link+`"`) {
                    t.Errorf("%s missing navigation destination %q", tc.path, link)
                }
            }
            if !strings.Contains(page, `<a class="skip-link" href="#main">Skip to content</a>`) {
                t.Errorf("%s has no keyboard skip navigation", tc.path)
            }
            if strings.Count(page, `<main id="main"`) != 1 {
                t.Errorf("%s should render exactly one main landmark", tc.path)
            }
            if !strings.Contains(page, tc.pageText) {
                t.Errorf("%s missing expected page content", tc.path)
            }
            if !strings.Contains(page, `<link rel="stylesheet" href="/assets/dashboard.css">`) {
                t.Errorf("%s missing common stylesheet", tc.path)
            }
        })
    }
}

func TestSharedSidebarKeepsSettingsFormsAndEventFilters(t *testing.T) {
    h, _, _, _ := setup(t)
    settings := request(h, http.MethodGet, "/settings", "", true)
    for _, form := range []string{
        `action="/settings/projects"`,
        `action="/settings/policies"`,
        `action="/settings/notifications"`,
        `name="csrf_token"`,
    } {
        if !strings.Contains(settings.Body.String(), form) {
            t.Errorf("settings lost %s", form)
        }
    }
    events := request(h, http.MethodGet, "/events?kind=remote_digest_changed&delivery=pending&search=hello", "", true)
    if events.Code != http.StatusOK {
        t.Fatalf("event filters GET returned %d", events.Code)
    }
    for _, value := range []string{
        `value="remote_digest_changed" selected`,
        `value="pending" selected`,
        `name="search" maxlength="128" value="hello"`,
        `action="/events"`,
    } {
        if !strings.Contains(events.Body.String(), value) {
            t.Errorf("event filter not preserved: %s", value)
        }
    }
}
