//go:build voice && cgo

package dave

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"sync"
	"testing"
)

func nativeTestConfig() Config {
	return Config{MaxParticipants: 8, MaxDecryptors: 8, MaxControlBytes: 65536,
		MaxProposalCount: 16, MaxProposalBytes: 65536, MaxPreparedTransitions: 8,
		MaxTransitionBytes: 65536, MaxFrameBytes: 1275, MaxEncryptedFrameBytes: 1536}
}

// Public SEC1 P256 generator + BasicCredential{00,01,01,00}. Serialization follows
// pinned MLSpp ExternalSender/SignaturePublicKey/Credential and the official
// cpp/test/external_sender.cpp credential. No private key or persisted fixture.
func externalSenderFixture(t *testing.T) []byte {
	t.Helper()
	p, err := hex.DecodeString("4041046b17d1f2e12c4247f8bce6e563a440f277037d812deb33a0f4a13945d898c2964fe342e2fe1a7f9b8ee7eb4a7c0f9e162bce33576b315ececbb6406837bf51f500010400010100")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func newNativeTestContext(t *testing.T) *Context {
	t.Helper()
	c, err := New(nativeTestConfig())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := c.Close(); err != nil {
			t.Error(err)
		}
	})
	return c
}

func TestNativeBoundsAndUnavailableMedia(t *testing.T) {
	if _, err := New(Config{}); !errors.Is(err, ErrConfig) {
		t.Fatalf("missing bounds: %v", err)
	}
	c := newNativeTestContext(t)
	ctx := context.Background()
	if c.Ready() {
		t.Fatal("new context published readiness")
	}
	if _, err := c.EncryptOpus(ctx, 1, []byte{0xf8, 0xff, 0xfe}, make([]byte, 1536)); !errors.Is(err, ErrNotReady) {
		t.Fatalf("media without key: %v", err)
	}
	if _, err := c.MaxCiphertextSize(1276); !errors.Is(err, ErrBounds) {
		t.Fatalf("unbounded frame: %v", err)
	}
	if err := c.Initialize(ctx, 0, "1", "2"); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("plaintext version: %v", err)
	}
	if err := c.Initialize(ctx, 1, "01", "2"); !errors.Is(err, ErrInput) {
		t.Fatalf("noncanonical identity: %v", err)
	}
	if err := c.SetExternalSender(ctx, make([]byte, nativeTestConfig().MaxControlBytes+1)); !errors.Is(err, ErrBounds) {
		t.Fatalf("native input allocation bound: %v", err)
	}
	if err := c.SetParticipants(ctx, []string{"2", "2"}); !errors.Is(err, ErrInput) {
		t.Fatalf("duplicate participant: %v", err)
	}
	if _, err := c.DecryptOpus(ctx, "3", []byte{1, 2, 3}, make([]byte, 1536)); !errors.Is(err, ErrUnknownSender) {
		t.Fatalf("unknown sender: %v", err)
	}
}

func TestNativeFreshPackagesAndIgnoredCommit(t *testing.T) {
	c := newNativeTestContext(t)
	ctx := context.Background()
	if err := c.Initialize(ctx, 1, "1", "2"); err != nil {
		t.Fatal(err)
	}
	first, err := c.FreshKeyPackage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	second, err := c.FreshKeyPackage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) == 0 || bytes.Equal(first, second) {
		t.Fatal("key package was reused")
	}
	// Upstream explicitly ignores an in-flight commit while there is neither
	// current state nor proposals, despite invoking its failure callback.
	result, err := c.ProcessCommit(ctx, 7, []byte{0xff})
	if err != nil || result != CommitIgnored {
		t.Fatalf("ignored became failed: %v %v", result, err)
	}
	if c.Ready() {
		t.Fatal("ignored commit published readiness")
	}
}

func TestNativePendingTransitionAndSoleMemberReset(t *testing.T) {
	c := newNativeTestContext(t)
	ctx := context.Background()
	if err := c.Initialize(ctx, 1, "1", "2"); err != nil {
		t.Fatal(err)
	}
	if err := c.PrepareTransition(ctx, 0, 1); !errors.Is(err, ErrNotReady) {
		t.Fatalf("group without sender: %v", err)
	}
	if err := c.SetExternalSender(ctx, externalSenderFixture(t)); err != nil {
		t.Fatal(err)
	}
	if _, err := c.FreshKeyPackage(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.SetParticipants(ctx, []string{"2", "3"}); err != nil {
		t.Fatal(err)
	}
	if err := c.AuthorizePendingReset(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if err := c.PrepareTransition(ctx, 0, 1); !errors.Is(err, ErrNotReady) {
		t.Fatalf("non-sole pending activation: %v", err)
	}
	// A failed prepare consumed the authorization. Model another authenticated
	// epoch-one reset and a successfully sent fresh package before authorizing.
	if err := c.Initialize(ctx, 1, "1", "2"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.FreshKeyPackage(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.SetParticipants(ctx, []string{"2"}); err != nil {
		t.Fatal(err)
	}
	if err := c.PrepareTransition(ctx, 0, 1); !errors.Is(err, ErrNotReady) {
		t.Fatalf("initial description/generated-only package authorized pending media: %v", err)
	}
	if err := c.AuthorizePendingReset(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if err := c.PrepareTransition(ctx, 0, 1); err != nil {
		t.Fatal(err)
	}
	if !c.Ready() || c.established {
		t.Fatal("pending media activation incorrectly established MLS or stayed unready")
	}
	opus := []byte{0xf8, 0xff, 0xfe}
	dst := make([]byte, nativeTestConfig().MaxEncryptedFrameBytes)
	n, err := c.EncryptOpus(ctx, 42, opus, dst)
	if err != nil || n <= len(opus) || bytes.Equal(dst[:n], opus) {
		t.Fatalf("real frame encryption failed: n=%d err=%v", n, err)
	}
	firstCiphertext := append([]byte(nil), dst[:n]...)
	if err := c.PrepareTransition(ctx, 0, 1); err != nil {
		t.Fatal(err)
	}
	n, err = c.EncryptOpus(ctx, 42, opus, dst)
	if err != nil || bytes.Equal(firstCiphertext, dst[:n]) {
		t.Fatalf("repeated zero reused sender nonce: %v", err)
	}
	// A reset prepares a new pending group but preserves the old active sender
	// until the gateway's immediate transition zero.
	oldGeneration := c.activeGeneration
	if err := c.Initialize(ctx, 1, "1", "2"); err != nil {
		t.Fatal(err)
	}
	if !c.Ready() || c.activeGeneration != oldGeneration {
		t.Fatal("reset switched sender before execute")
	}
	if _, err := c.FreshKeyPackage(ctx); err != nil {
		t.Fatal(err)
	}
	if err := c.PrepareTransition(ctx, 0, 1); !errors.Is(err, ErrNotReady) {
		t.Fatalf("previous generation authorized a fresh pending reset: %v", err)
	}
	if err := c.AuthorizePendingReset(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if err := c.PrepareTransition(ctx, 0, 1); err != nil {
		t.Fatal(err)
	}
	if c.activeGeneration == oldGeneration || c.established {
		t.Fatal("sole member reset did not activate fresh pending media")
	}
	if _, err := c.EncryptOpus(ctx, 43, opus, dst); !errors.Is(err, ErrInput) {
		t.Fatalf("unbounded SSRC codec map: %v", err)
	}
}

func TestNativeEmptyProposalAndFailedCommit(t *testing.T) {
	c := newNativeTestContext(t)
	ctx := context.Background()
	if err := c.Initialize(ctx, 1, "1", "2"); err != nil {
		t.Fatal(err)
	}
	if err := c.SetExternalSender(ctx, externalSenderFixture(t)); err != nil {
		t.Fatal(err)
	}
	// An empty TLS list must not synthesize a local commit or activate media.
	out, err := c.ProcessProposals(ctx, ProposalAppend, []byte{0}, []string{"2"})
	if err != nil || len(out.Wire) != 0 || c.Ready() {
		t.Fatalf("empty proposal result: %d %v", len(out.Wire), err)
	}
	result, err := c.ProcessCommit(ctx, 9, []byte{0xff})
	if result != CommitFailed || !errors.Is(err, ErrNative) {
		t.Fatalf("invalid commit not failed: %v %v", result, err)
	}
	// Reject a truncated vector in the single native parser; do not silently
	// accept its framing or return a locally fabricated commit.
	if _, err := c.ProcessProposals(ctx, ProposalAppend, []byte{4, 1}, []string{"2"}); !errors.Is(err, ErrNative) {
		t.Fatalf("malformed proposal framing: %v", err)
	}
}

func TestNativeCloseAndSynchronousCallbackOwnership(t *testing.T) {
	c := newNativeTestContext(t)
	ctx := context.Background()
	if err := c.Initialize(ctx, 1, "1", "2"); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Invalid external sender enters the real synchronous native MLS
			// failure callback while Close competes for the same owner lock.
			err := c.SetExternalSender(ctx, []byte{0xff})
			if !errors.Is(err, ErrNative) && !errors.Is(err, ErrClosed) {
				t.Errorf("callback boundary: %v", err)
			}
		}()
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if c.Ready() {
		t.Fatal("closed native context remains ready")
	}
	if _, err := c.FreshKeyPackage(ctx); !errors.Is(err, ErrClosed) {
		t.Fatalf("use after close: %v", err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := c.Initialize(canceled, 1, "1", "2"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled native admission: %v", err)
	}
}
