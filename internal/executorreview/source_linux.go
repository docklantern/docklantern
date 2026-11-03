package executorreview

import (
	"context"
	"time"

	"github.com/docklantern/docklantern/internal/executorproposal"
	"github.com/docklantern/docklantern/internal/executorregistration"
)

// ProtectedSource wires the two existing safe readers together. The monitor
// UID must come from independent host-admin configuration, never the proposal
// or web app. No production InspectActual implementation is provided until
// the exact-version and stable-Compose preflight in B6-04 is accepted.
type ProtectedSource struct {
	TrustedMonitorUID uint32
	Inspector func(context.Context, executorregistration.Record, string) (Baseline, error)
}

func (s ProtectedSource) LoadProposal(id string, now time.Time) (executorproposal.Record, error) {
	if s.TrustedMonitorUID == 0 { return executorproposal.Record{}, ErrReviewDenied }
	return executorproposal.LoadInbox(id, s.TrustedMonitorUID, now)
}

func (s ProtectedSource) LoadRegistration(id string) (executorregistration.Record, error) {
	return executorregistration.LoadProtected(id)
}

func (s ProtectedSource) InspectActual(ctx context.Context, reg executorregistration.Record, service string) (Baseline, error) {
	if s.Inspector == nil { return Baseline{}, ErrReviewDenied }
	return s.Inspector(ctx, reg, service)
}

var _ Source = ProtectedSource{}
