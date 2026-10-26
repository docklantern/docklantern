package discovery

const monitorLabel = "com.docklantern.monitor"
const intervalLabel = "com.docklantern.interval"

// Only pass explicitly supported keys into the monitoring reconciliation
// layer; Docker labels may contain unrelated information or secrets.
func monitorLabels(labels map[string]string) map[string]string {
    var out map[string]string
    for _, key := range []string{monitorLabel, intervalLabel} {
        if value, ok := labels[key]; ok {
            if out == nil { out = make(map[string]string,2) }
            out[key] = value
        }
    }
    return out
}
