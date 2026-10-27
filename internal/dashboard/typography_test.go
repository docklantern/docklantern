package dashboard

import (
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
)

// Styles are embedded, local and shared by all five monitoring workspaces.
// This regression check cannot replace a human desktop/mobile visual review.
func TestReadableLocalDashboardTypography(t *testing.T){
 r:=httptest.NewRecorder()
 Handler(fakeReader{}).ServeHTTP(r,httptest.NewRequest(http.MethodGet,"/assets/dashboard.css",nil))
 if r.Code!=http.StatusOK{t.Fatalf("CSS returned %d",r.Code)}
 css:=r.Body.String()
 for _,rule:=range []string{
  "font-family: Inter",
  "font-size: 15px;",
  "--muted: #b1c4da;",
  "font-size: 13.5px;",
  "font-size: 12.5px;",
  ".sidebar-foot .build-version",
 }{
  if !strings.Contains(css,rule){t.Errorf("missing legible typography rule: %s",rule)}
 }
 if strings.Contains(css,"@import url(") || strings.Contains(css,"fonts.googleapis.com"){
  t.Fatal("dashboard should not depend on remote web fonts")
 }
}
