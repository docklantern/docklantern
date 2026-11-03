package executorcli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/docklantern/docklantern/internal/executorpreview"
	"github.com/docklantern/docklantern/internal/executorregistration"
)

func testPreview(id string) (executorpreview.View,error) {
	if id!="plan-demo" {return executorpreview.View{}, errors.New("unexpected proposal")}
	return executorpreview.View{
		ProposalID: id, RegistrationID:"demo-registration", Project:"hello-go",
		Service:"web", Repository:"docker.io/example/hello-go",
		RequestedTag:"latest", CandidateDigest:"sha256:"+strings.Repeat("a",64),
		Platform:"linux/amd64", ExpiresAt:"2026-11-03T05:10:00Z",
		ProposalSHA256:"sha256:"+strings.Repeat("b",64),
		RegistrationSHA256:"sha256:"+strings.Repeat("c",64),
	},nil
}

func unusedRegister(string,string) (executorregistration.Record,error) {
	return executorregistration.Record{},errors.New("registration must never run")
}

func TestPreviewCLIIsReadOnlyAndExplicitlyUnqualified(t *testing.T) {
	var output,errOut bytes.Buffer
	code:=RunWithPreview([]string{"preview","plan-demo"},&output,&errOut,"dev",
		disabledDiagnostic,unusedRegister,testPreview)
	if code!=ExitOK || errOut.Len()!=0 {
		t.Fatalf("preview failed: code=%d stderr=%q",code,errOut.String())
	}
	for _,marker:=range []string{
		"READ ONLY","plan-demo","demo-registration","hello-go",
		"Actual Docker/container baseline: NOT INSPECTED",
		"OCI digest and platform authenticity: NOT VERIFIED",
		"DEPLOYMENT AUTHORIZATION: NOT GRANTED",
	} {
		if !strings.Contains(output.String(),marker) {t.Errorf("missing preview safety marker %q",marker)}
	}
}

func TestPreviewRejectsInvalidCLIWithoutCallingReader(t *testing.T) {
	for _,args:=range [][]string{
		{"preview"}, {"preview","../etc/shadow"}, {"preview","-"},
		{"preview","plan-demo","--yes"}, {"preview","plan-demo","apply"},
		{"preview","plan-demo","--monitor-uid","65534"},
		{"preview","plan-demo","--docker-host","tcp://evil:2375"},
		{"preview","plan-demo","--file","/etc/shadow"},
		{"preview","plan-demo\nsecret"},
	} {
		calls:=0
		var stdout,stderr bytes.Buffer
		code:=RunWithPreview(args,&stdout,&stderr,"dev",disabledDiagnostic,unusedRegister,
			func(string)(executorpreview.View,error){
				calls++
				return executorpreview.View{},nil
			})
		if code!=ExitUsage || calls!=0 || stdout.Len()!=0 ||
			strings.Contains(stderr.String(),"/etc/") {
			t.Errorf("invalid preview invoked reader args=%q code=%d calls=%d stdout=%q stderr=%q",
				args,code,calls,stdout.String(),stderr.String())
		}
	}
}

func TestPreviewSanitizesFileAndDockerErrors(t *testing.T) {
	var stdout,stderr bytes.Buffer
	code:=RunWithPreview([]string{"preview","plan-demo"},&stdout,&stderr,
		"dev",disabledDiagnostic,unusedRegister,
		func(string)(executorpreview.View,error){
			return executorpreview.View{}, errors.New("secret=private /var/lib/docklantern/executor")
		})
	if code!=ExitUnavailable || stdout.Len()!=0 ||
		!strings.Contains(stderr.String(),"could not verify protected inputs") ||
		strings.Contains(stderr.String(),"secret") ||
		strings.Contains(stderr.String(),"/var/lib/") {
		t.Fatalf("leaked preview error: code=%d stdout=%q stderr=%q",code,stdout.String(),stderr.String())
	}
}

func TestPreviewDoesNotActivateDockerMutationCommands(t *testing.T) {
	calls:=0
	for _,args:=range [][]string{
		{"apply"}, {"apply","plan-demo"}, {"recover"}, {"reconcile"},
		{"status"}, {"apply","--yes"}, {"recover","--yes"},
	} {
		var output,stderr bytes.Buffer
		code:=RunWithPreview(args,&output,&stderr,"dev",disabledDiagnostic,unusedRegister,
			func(string)(executorpreview.View,error){
				calls++
				return executorpreview.View{},nil
			})
		if code==ExitOK || output.Len()!=0 || calls!=0 {
			t.Fatalf("mutation command became active args=%q code=%d calls=%d",
				args,code,calls)
		}
	}
}

func TestPreviewHelpDistinguishesObservationFromAuthorization(t *testing.T) {
	var out,stderr bytes.Buffer
	code:=RunWithPreview([]string{"help"},&out,&stderr,"dev",disabledDiagnostic,unusedRegister,nil)
	if code!=ExitOK || !strings.Contains(out.String(),"No actual Docker/Compose/OCI inspection") ||
		!strings.Contains(out.String(),"No deployment") {
		t.Fatalf("help did not disclose limits: %d %q",code,out.String())
	}
}
