package presentation

import (
 "sort"
 "time"
)

// ImageOverviewMetrics use the same image/platform groups and evidence rules
// as the Images workspace. All counts are unique image identities, not
// container replicas, individual checks, or inferred software releases.
type ImageOverviewMetrics struct {
 WatchedImages int
 RemoteChanges24h int
 DeploymentDifferences int
 MonitoringErrors int
}

type ImageReview struct {
 Image ImageSummary
 Reason string
 Priority int
}

func RecentRecordedChange(image ImageSummary, now time.Time, window time.Duration) bool {
 if image.LastChangeAt == "" || window <= 0 { return false }
 at, err := time.Parse(time.RFC3339Nano, image.LastChangeAt)
 if err != nil { return false }
 age := now.Sub(at)
 return age >= 0 && age <= window
}

// BuildImageOverview never interprets a remote digest movement as a newer
// software version. Review rows are prioritized by actual monitoring failures,
// verified digest differences, retained changes, then stale observations.
func BuildImageOverview(images []ImageSummary, now time.Time, reviewLimit int) (ImageOverviewMetrics, []ImageReview, int) {
 metrics := ImageOverviewMetrics{WatchedImages: len(images)}
 review := make([]ImageReview, 0, len(images))
 for _, image := range images {
  changed := RecentRecordedChange(image, now, 24*time.Hour)
  if changed { metrics.RemoteChanges24h++ }
  if image.DeploymentCounts.Different > 0 { metrics.DeploymentDifferences++ }
  if image.FailedServices > 0 { metrics.MonitoringErrors++ }
  item := ImageReview{Image:image}
  switch {
  case image.FailedServices > 0:
   item.Reason = "Registry monitoring error"
   item.Priority = 0
  case image.DeploymentCounts.Different > 0:
   item.Reason = "Verified deployed digest differs"
   item.Priority = 1
  case changed:
   item.Reason = "Remote digest changed in the last 24 hours"
   item.Priority = 2
  case image.RegistryStatus == "stale":
   item.Reason = "Registry observation is stale"
   item.Priority = 3
  default:
   continue
  }
  review = append(review,item)
 }
 sort.SliceStable(review,func(i,j int)bool{
  if review[i].Priority != review[j].Priority {return review[i].Priority < review[j].Priority}
  if review[i].Image.Reference != review[j].Image.Reference {
   return review[i].Image.Reference < review[j].Image.Reference
  }
  return review[i].Image.Platform < review[j].Image.Platform
 })
 totalReview := len(review)
 if reviewLimit < 0 { reviewLimit=0 }
 if len(review)>reviewLimit {review=review[:reviewLimit]}
 return metrics, review, totalReview
}
