package updates

import (
 "testing"
)
func TestRepositoryIdentity(t *testing.T){
 // Matching a pinned digest from another image must never establish evidence.
 c:="sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
 if _,ok:=pinnedDigest("ghcr.io/team/api@"+c,"ghcr.io/team/worker:latest");ok{t.Fatal("cross-repository digest accepted")}
 if got,ok:=pinnedDigest("docker.io/library/nginx@"+c,"nginx:latest");!ok||got!=c{t.Fatalf("canonical Docker Hub identity failed %q %v",got,ok)}
 if _,ok:=pinnedDigest("nginx@"+c,"nginx@"+c);ok{t.Fatal("watched policy should not be digest-pinned")}
}
