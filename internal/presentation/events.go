// Package presentation owns human-readable, read-only monitoring event labels.
// Events are observations, not deployed updates or confirmed newer releases.
package presentation

import (
 "fmt"

 "github.com/docklantern/docklantern/internal/store"
)

func EventKind(kind string) string {
 switch kind {
 case "remote_digest_changed": return "Remote registry digest changed"
 case "registry_failure": return "Registry check failed"
 case "registry_recovered": return "Registry check recovered"
 case "registry_outage_prolonged": return "Registry outage prolonged"
 default: return kind
 }
}

func EventScope(e store.Event) string {
 if e.Project!="" && e.Service!="" {return e.Project+" / "+e.Service}
 if e.Image!="" {return "Registry image event (image/platform scope)"}
 return "Monitoring event"
}

func EventDelivery(e store.Event) string {
 if e.DeliveryState=="pending" {
  return fmt.Sprintf("Webhook pending (%d attempts)",e.DeliveryAttempts)
 }
 if e.DeliveryState=="delivered" {
  return fmt.Sprintf("Webhook delivered (%d attempts)",e.DeliveryAttempts)
 }
 return e.DeliveryState
}
