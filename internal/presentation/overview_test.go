package presentation

import (
 "reflect"
 "testing"
 "time"
)

func TestImageOverviewMetricsAndPriority(t *testing.T){
 now:=time.Date(2026,10,26,19,0,0,0,time.UTC)
 images:=[]ImageSummary{
  {Reference:"z.example/old:1",Platform:"linux/amd64",RegistryStatus:"unchanged",
   LastChangeAt:now.Add(-48*time.Hour).Format(time.RFC3339),
   DeploymentStatus:"current",DeploymentCounts:DeploymentCounts{Current:1}},
  {Reference:"a.example/shared:latest",Platform:"linux/amd64",RegistryStatus:"unchanged",
   LastChangeAt:now.Add(-time.Hour).Format(time.RFC3339Nano),
   DeploymentStatus:"different",DeploymentCounts:DeploymentCounts{Different:2}},
  {Reference:"a.example/shared:latest",Platform:"linux/arm64",RegistryStatus:"check_failed",
   FailedServices:2,DeploymentStatus:"unknown",DeploymentCounts:DeploymentCounts{Unknown:1}},
  {Reference:"b.example/one:latest",Platform:"linux/amd64",RegistryStatus:"changed",
   LastChangeAt:now.Add(-2*time.Hour).Format(time.RFC3339),
   DeploymentStatus:"unknown",DeploymentCounts:DeploymentCounts{Unknown:1}},
  {Reference:"b.example/slow:latest",Platform:"linux/amd64",RegistryStatus:"stale",
   DeploymentStatus:"unknown",DeploymentCounts:DeploymentCounts{Unknown:1}},
  {Reference:"c.example/paused:latest",Platform:"linux/amd64",RegistryStatus:"unchanged",
   DeploymentStatus:"not_running",DeploymentCounts:DeploymentCounts{NotRunning:1}},
 }
 metrics,review,total:=BuildImageOverview(images,now,3)
 if metrics.WatchedImages!=6 || metrics.RemoteChanges24h!=2 || metrics.DeploymentDifferences!=1 || metrics.MonitoringErrors!=1 {
  t.Fatalf("unexpected metrics: %+v",metrics)
 }
 if total!=4 || len(review)!=3 {t.Fatalf("unexpected review size: total=%d, page=%d",total,len(review))}
 got:=[]string{}
 for _,item:=range review {got=append(got,item.Reason)}
 want:=[]string{"Registry monitoring error","Verified deployed digest differs","Remote digest changed in the last 24 hours"}
 if !reflect.DeepEqual(got,want){t.Fatalf("review reasons got %v want %v",got,want)}
 if images[0].Reference!="z.example/old:1" {t.Fatal("input images mutated")}
 _,allReview,allCount:=BuildImageOverview(images,now,10)
 if len(allReview)!=4 || allCount!=4 {t.Fatal("review total not preserved")}
 if allReview[3].Reason!="Registry observation is stale" {t.Fatalf("missing stale review row: %+v",allReview[3])}
 _,none,stillTotal:=BuildImageOverview(images,now,0)
 if len(none)!=0 || stillTotal!=4{t.Fatal("bounded review limit ignored")}
}

func TestRecentRecordedChangeBoundaryAndInvalid(t *testing.T){
 now:=time.Date(2026,10,26,19,0,0,0,time.UTC)
 cases:=[]struct{stamp string;want bool}{
  {now.Format(time.RFC3339),true},
  {now.Add(-24*time.Hour).Format(time.RFC3339),true},
  {now.Add(-24*time.Hour-time.Nanosecond).Format(time.RFC3339Nano),false},
  {now.Add(time.Minute).Format(time.RFC3339),false},
  {"garbage",false},
  {"",false},
 }
 for _,tc:=range cases{
  if got:=RecentRecordedChange(ImageSummary{LastChangeAt:tc.stamp},now,24*time.Hour);got!=tc.want{
   t.Errorf("timestamp %q got %v want %v",tc.stamp,got,tc.want)
  }
 }
}
