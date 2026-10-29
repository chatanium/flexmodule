//go:build voice && cgo

package dave

import (
	"context"
	"errors"
	"testing"
)

// This native-boundary fixture models the trusted Gateway owner's explicit
// authenticated epoch-one reset and successful opcode-26 write. Transport tests
// separately own the wire observation; generating a package never grants this.
func authorizeNativeTestReset(t *testing.T, c *Context) {
	t.Helper()
	ctx := context.Background()
	if err := c.Initialize(ctx, 1, "1", "2"); err != nil {
		t.Fatal(err)
	}
	if err := c.SetExternalSender(ctx, externalSenderFixture(t)); err != nil {
		t.Fatal(err)
	}
	if err := c.SetParticipants(ctx, []string{"2"}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.FreshKeyPackage(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.AuthorizePendingReset(ctx, 1); err != nil {
		t.Fatal(err)
	}
}

func TestNativePendingResetAuthorityRevocation(t *testing.T) {
	cases := []struct {
		name      string
		wantError bool
		invoke    func(*Context) error
	}{
		{"same-version-epoch-supersedes", false, func(c *Context) error { return c.SetProtocolVersion(context.Background(), 1) }},
		{"fresh-unsent-package", false, func(c *Context) error { _, err := c.FreshKeyPackage(context.Background()); return err }},
		{"new-reset-generation", false, func(c *Context) error { return c.Initialize(context.Background(), 1, "1", "2") }},
		{"proposal-error", true, func(c *Context) error {
			_, err := c.ProcessProposals(context.Background(), ProposalOperation(2), []byte{0}, []string{"2"})
			return err
		}},
		{"welcome-error", true, func(c *Context) error { return c.ProcessWelcome(context.Background(), 9, []byte{0xff}, []string{"2"}) }},
		{"prepare-error", true, func(c *Context) error { return c.PrepareReceivers(context.Background(), 9) }},
		{"execute-error", true, func(c *Context) error { return c.ExecuteTransition(context.Background(), 9) }},
		{"participant-error", true, func(c *Context) error { return c.SetParticipants(context.Background(), []string{"2", "2"}) }},
		{"unsupported-transition", true, func(c *Context) error { return c.PrepareTransition(context.Background(), 0, 0) }},
		{"external-sender-error", true, func(c *Context) error { return c.SetExternalSender(context.Background(), []byte{0xff}) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newNativeTestContext(t)
			authorizeNativeTestReset(t, c)
			if err := tc.invoke(c); (err != nil) != tc.wantError {
				t.Fatalf("control result does not match its revocation case: %v", err)
			}
			if c.pendingResetGeneration != 0 || c.pendingResetVersion != 0 {
				t.Fatal("superseded or rejected control retained pending activation authority")
			}
			if err := c.PrepareTransition(context.Background(), 0, 1); !errors.Is(err, ErrNotReady) {
				t.Fatalf("revoked reset activated pending media: %v", err)
			}
		})
	}
}

func TestNativeIgnoredCommitPreservesResetAuthorityButFailureRevokes(t *testing.T) {
	c := newNativeTestContext(t)
	authorizeNativeTestReset(t, c)
	ctx := context.Background()
	result, err := c.ProcessCommit(ctx, 8, []byte{0xff})
	if err != nil || result != CommitIgnored || c.pendingResetGeneration != c.generation {
		t.Fatalf("ignored commit incorrectly revoked the sanctioned reset: %v %v", result, err)
	}
	out, err := c.ProcessProposals(ctx, ProposalAppend, []byte{0}, []string{"2"})
	if err != nil || len(out.Wire) != 0 {
		t.Fatalf("empty native proposal list: %v", err)
	}
	result, err = c.ProcessCommit(ctx, 8, []byte{0xff})
	if result != CommitFailed || !errors.Is(err, ErrNative) || c.pendingResetGeneration != 0 {
		t.Fatalf("failed commit retained activation authority: %v %v", result, err)
	}
}

func TestNativeDelayedExternalSenderPreservesSentPackageGeneration(t *testing.T) {
	c := newNativeTestContext(t)
	ctx := context.Background()
	if err := c.Initialize(ctx, 1, "1", "2"); err != nil {
		t.Fatal(err)
	}
	if err := c.SetParticipants(ctx, []string{"2"}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.FreshKeyPackage(ctx); err != nil {
		t.Fatal(err)
	}
	sentGeneration := c.keyPackageGeneration
	// The Gateway owner retains its authenticated epoch-one/successful-write
	// record while waiting for opcode 25; native generation alone grants nothing.
	if c.pendingResetGeneration != 0 {
		t.Fatal("package generation granted authority")
	}
	if err := c.SetExternalSender(ctx, externalSenderFixture(t)); err != nil {
		t.Fatal(err)
	}
	if c.keyPackageGeneration != sentGeneration || c.generation != sentGeneration {
		t.Fatal("delayed external sender invalidated the existing native join package")
	}
	if err := c.AuthorizePendingReset(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if err := c.PrepareTransition(ctx, 0, 1); err != nil {
		t.Fatal(err)
	}
	if !c.Ready() || c.established || c.pendingResetGeneration != 0 {
		t.Fatal("delayed sender did not produce one consumed pending-media activation")
	}
}
