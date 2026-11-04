package settings

import (
 "bytes"
 "strings"
 "testing"

 "github.com/docklantern/docklantern/internal/presentation"
)

func TestServiceDeploymentReviewDoesNotImplyApproval(t *testing.T) {
 service:=presentation.ServiceSummary{
  ServiceIdentity:presentation.ServiceIdentity{Project:"demo",Service:"web"},
  Image:"docker.io/example/web:latest",
  HasPolicy:true,
  Monitoring:"monitoring",
  Enrollment:"Docker label",
 }
 data:=serviceDetailsData{
  workspaceData:workspaceData{Service:&service},
  RemoteDigest:"sha256:"+strings.Repeat("a",64),
  DeployedDigest:"sha256:"+strings.Repeat("b",64),
 }
 var b bytes.Buffer
 if err:=serviceExperienceDetailTemplate.Execute(&b,data);err!=nil{
  t.Fatalf("render service details: %v",err)
 }
 html:=b.String()
 for _,text:=range []string{
  "Deployment review · read-only",
  "Host executor registration",
  "Not verified by this GUI",
  "Unknown · requires host qualification",
  "Registry evidence only.",
  "Not available without independent executor qualification",
  "Host-local only",
  "historical success is not a live Docker attestation",
  "docker.io/example/web:latest",
  "sha256:"+strings.Repeat("a",64),
  "sha256:"+strings.Repeat("b",64),
  "Monitoring configuration",
  "Watched registry tag",
  "Registry check",
 }{
  if !strings.Contains(html,text){t.Errorf("missing deployment boundary/evidence: %q",text)}
 }
 for _,forbidden:=range []string{
  "action=\"/deployments/apply\"",
  "action=\"/executor",
  "name=\"candidate_digest\"",
  "name=\"proposal_id\"",
 }{
  if strings.Contains(html,forbidden){t.Errorf("untrusted GUI must not expose executor action: %s",forbidden)}
 }
}

func TestServiceDeploymentReviewNoWatchedCandidate(t *testing.T) {
 service:=presentation.ServiceSummary{
  ServiceIdentity:presentation.ServiceIdentity{Project:"demo",Service:"web"},
  Monitoring:"not_enrolled",
 }
 var b bytes.Buffer
 err:=serviceExperienceDetailTemplate.Execute(&b,serviceDetailsData{
  workspaceData:workspaceData{Service:&service},
 })
 if err!=nil{t.Fatalf("render unenrolled service: %v",err)}
 html:=b.String()
 for _,text:=range []string{
  "No watched reference","Not observed",
  "Not verified","Unknown · requires host qualification",
 }{
  if !strings.Contains(html,text){t.Errorf("unenrolled service missing conservative state %q",text)}
 }
 if strings.Contains(html,"Verified deployment eligible"){
  t.Fatal("missing registration or candidate must not become eligible")
 }
}
