//go:build voice && cgo

package dave

/*
#include "bridge.h"
*/
import "C"

import (
	"context"
	"runtime"
	"unsafe"
)

// ProcessProposals receives the TLS proposal/ref list excluding its operation
// byte. The source-patched official parser enforces exact queued proposal count
// and serialized-byte limits in its single parse/validation path.
// On success, empty Wire means the actual native proposal queue is empty and
// its outbound candidate was cleared. A nonempty queue synchronously produces
// its candidate; native failures return errors, never successful deferred output.
func (c *Context) ProcessProposals(ctx context.Context, operation ProposalOperation, proposals []byte, recognizedIDs []string) (out CommitWelcome, resultErr error) {
	if err := c.enter(ctx); err != nil {
		return CommitWelcome{}, err
	}
	defer c.mu.Unlock()
	defer c.clearPendingResetOnError(&resultErr)
	if !c.initialized {
		return CommitWelcome{}, ErrNotReady
	}
	if operation != ProposalAppend && operation != ProposalRevoke {
		return CommitWelcome{}, ErrInput
	}
	if len(proposals) == 0 {
		return CommitWelcome{}, ErrInput
	}
	if len(proposals) >= c.cfg.MaxControlBytes || len(proposals) >= c.cfg.MaxProposalBytes {
		return CommitWelcome{}, ErrBounds
	}
	if err := c.setParticipants(recognizedIDs); err != nil {
		return CommitWelcome{}, err
	}
	recognized, release, err := recognizedArray(recognizedIDs)
	if err != nil {
		return CommitWelcome{}, err
	}
	defer release()
	size := len(proposals) + 1
	if len(c.proposalScratch) < size {
		c.proposalScratch = make([]byte, size)
	}
	wire := c.proposalScratch[:size]
	wire[0] = byte(operation)
	copy(wire[1:], proposals)
	var p *C.uint8_t
	var n C.size_t
	c.resetFailure()
	C.daveSessionProcessProposals(c.session, pointer(wire), C.size_t(size), recognized,
		C.size_t(len(recognizedIDs)), &p, &n)
	runtime.KeepAlive(wire)
	if err := c.checkNative(); err != nil {
		if p != nil {
			C.daveFree(unsafe.Pointer(p))
		}
		return CommitWelcome{}, err
	}
	result, err := c.copyOutput(p, n, false)
	if err != nil {
		return CommitWelcome{}, err
	}
	return CommitWelcome{Wire: result}, nil
}
