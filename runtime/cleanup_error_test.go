package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/thirdscam/chatanium-flexmodule/internal/host"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestCleanupFailurePhasesAreFiniteOrderedAndPreserveCauses(t *testing.T) {
	const secret = "https://credential.invalid/RAW_SECRET"
	hostCause := &host.Error{Reason: "HOST_OPERATION_FAILED", Message: secret}
	timeout := fmt.Errorf("unsafe %s: %w", secret, context.DeadlineExceeded)
	joined := errors.Join(
		cleanupPhaseError(CleanupHostRelease, hostCause),
		errors.Join(cleanupPhaseError(CleanupStopHook, timeout), cleanupPhaseError(CleanupHostRelease, hostCause)),
	)
	phases := CleanupFailurePhases(joined)
	if len(phases) != 2 || phases[0] != CleanupStopHook || phases[1] != CleanupHostRelease {
		t.Fatalf("phase ordering/dedup changed: %v", phases)
	}
	if !errors.Is(joined, context.DeadlineExceeded) {
		t.Fatal("cleanup wrapper lost context error identity")
	}
	var extracted *host.Error
	if !errors.As(joined, &extracted) || extracted != hostCause {
		t.Fatal("cleanup wrapper lost original typed host error")
	}
	if strings.Contains(joined.Error(), secret) || strings.Contains(joined.Error(), "RAW_SECRET") {
		t.Fatal("cleanup phase diagnostics copied source error text")
	}
	original := status.Error(codes.PermissionDenied, secret)
	wrapped := cleanupPhaseError(CleanupHelperClose, original)
	if status.Code(wrapped) != status.Code(original) {
		t.Fatal("phase attribution changed the original gRPC status")
	}
}

type cleanupCauseProbe struct {
	visited *bool
	inner   error
}

func (*cleanupCauseProbe) Error() string       { return "RAW_SECRET" }
func (probe *cleanupCauseProbe) Unwrap() error { *probe.visited = true; return probe.inner }

func TestCleanupFailurePhasesDoNotInspectOpaqueBackendCauses(t *testing.T) {
	visited := false
	cause := &cleanupCauseProbe{visited: &visited, inner: cleanupPhaseError(CleanupTransport, errors.New("RAW_SECRET"))}
	phases := CleanupFailurePhases(cleanupPhaseError(CleanupHostTerminate, cause))
	if visited || len(phases) != 1 || phases[0] != CleanupHostTerminate {
		t.Fatal("phase accessor inspected or attributed opaque backend cause internals")
	}
	for _, err := range []error{
		nil,
		errors.New("RAW_SECRET"),
		cleanupPhaseError(CleanupHostRelease, nil),
		cleanupPhaseError(CleanupPhase(255), errors.New("RAW_SECRET")),
		runtimeError(codes.Unavailable, "CLEANUP_INCOMPLETE"),
	} {
		if len(CleanupFailurePhases(err)) != 0 {
			t.Fatal("phase accessor fabricated a failed or skipped step")
		}
	}
	if CleanupPhase(255).String() != "UNKNOWN" {
		t.Fatal("unknown phase escaped the fixed diagnostic vocabulary")
	}
}
