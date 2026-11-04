package dashboard

import (
 "bytes"
 "strings"
 "testing"

 "github.com/docklantern/docklantern/internal/deploymentactivity"
)

func renderDeploymentRow(t *testing.T, row deploymentactivity.Row) string {
 t.Helper()
 var out bytes.Buffer
 err := deploymentPage.ExecuteTemplate(&out, "deployments.html", deploymentView{
  ActivePage: "deployments",
  Rows: []deploymentactivity.Row{row},
 })
 if err != nil { t.Fatalf("render deployment page: %v", err) }
 return out.String()
}

func TestDeploymentActivityStaleReceiptCannotClaimCurrentSuccess(t *testing.T) {
 for _, outcome := range []string{"succeeded", "pending", "needs_reconciliation"} {
  t.Run(outcome, func(t *testing.T) {
   page := renderDeploymentRow(t, deploymentactivity.Row{
    OperationID: "operation-demo",
    Service: "web",
    Phase: "COMMIT",
    Outcome: outcome,
    CandidateDigest: "sha256:0123456789",
    UpdatedAt: "2026-11-01T10:00:00Z",
    Stale: true,
   })
   if !strings.Contains(page, "Stale receipt · current deployment state unknown") {
    t.Fatal("stale receipt did not explicitly mark current state unknown")
   }
   if strings.Contains(page, "Verified update committed at receipt time</span>") ||
    strings.Contains(page, "Needs manual reconciliation</span>") ||
    strings.Contains(page, "Pending · not verified successful</span>") {
    t.Fatal("stale receipt was presented as a fresh outcome")
   }
   if !strings.Contains(page, "pill warn") {
    t.Fatal("stale status must not use success styling")
   }
  })
 }
}

func TestDeploymentActivityFreshOutcomeWording(t *testing.T) {
 cases := []struct {
  outcome string
  expected string
  pill string
 }{
  {"succeeded", "Verified update committed at receipt time", "pill ok"},
  {"pending", "Pending · not verified successful", "pill muted"},
  {"needs_reconciliation", "Needs manual reconciliation", "pill warn"},
 }
 for _, tc := range cases {
  t.Run(tc.outcome, func(t *testing.T) {
   page := renderDeploymentRow(t, deploymentactivity.Row{
    OperationID: "operation-demo", Service: "web", Phase: "COMMIT",
    Outcome: tc.outcome, CandidateDigest: "sha256:0123456789",
    UpdatedAt: "2026-11-04T18:00:00Z",
   })
   if !strings.Contains(page, tc.expected) || !strings.Contains(page, tc.pill) {
    t.Fatalf("unexpected fresh %s receipt presentation", tc.outcome)
   }
   if strings.Contains(page, "Stale receipt ·") {
    t.Fatal("fresh receipt labeled stale")
   }
  })
 }
}
