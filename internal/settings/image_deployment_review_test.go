package settings

import (
 "bytes"
 "strings"
 "testing"

 "github.com/docklantern/docklantern/internal/presentation"
)

func TestImageDetailsKeepsDeploymentReviewNonExecutable(t *testing.T) {
 image:=presentation.ImageSummary{
  Reference:"registry-1.docker.io/library/example:latest",
  Platform:"linux/amd64",
  RegistryStatus:"changed",
  DeploymentStatus:"different",
  Services:[]presentation.ServiceIdentity{{Project:"demo",Service:"web"}},
 }
 svc:=presentation.ServiceSummary{
  ServiceIdentity:presentation.ServiceIdentity{Project:"demo",Service:"web"},
  Monitoring:"monitoring",Assessment:"different",
 }
 data:=imageDetailsData{
  workspaceData:workspaceData{Image:&image},
  Evidence:[]imageServiceEvidence{{Service:svc}},
 }
 var rendered bytes.Buffer
 if err:=imageExperienceDetailTemplate.Execute(&rendered,data);err!=nil{
  t.Fatalf("render image details: %v",err)
 }
 html:=rendered.String()
 for _,required:=range []string{
  "Deployment candidates · informational only",
  "host executor registration",
  "Each related service must be independently qualified",
  "Review deployment requirements",
  "/services/demo/web#deployment-review",
  "Host registration and immutable proposal not verified here",
  "/deployments",
  "Registry check",
  "Registry evidence",
  "Associated services",
 }{
  if !strings.Contains(html,required){t.Errorf("missing read-only workflow text %q",required)}
 }
 for _,forbidden:=range []string{
  "action=\"/executor",
  "action=\"/deployments/apply",
  "name=\"proposal_id\"",
  "name=\"candidate_digest\"",
 }{
  if strings.Contains(html,forbidden){t.Errorf("image detail must not authorize executor: %s",forbidden)}
 }
}
