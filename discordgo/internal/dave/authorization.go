//go:build voice && cgo

package dave

import "context"

func (c *Context) clearPendingReset() {
	c.pendingResetGeneration = 0
	c.pendingResetVersion = 0
}

// clearPendingResetOnError runs before the admitted control operation releases
// its owner lock. Rejected control state cannot leave a reusable activation grant.
func (c *Context) clearPendingResetOnError(err *error) {
	if *err != nil {
		c.clearPendingReset()
	}
}

// AuthorizePendingReset binds a single pending-self activation to the current
// native generation, version and latest one-use key package. The Gateway owner
// MUST call it only after an authenticated opcode 24 with epoch=1 reset this
// generation AND its fresh opcode-26 package was successfully written. Initial
// Session Description and invalid-commit recovery alone do not authorize it.
// It is not an acknowledgement that a package merely generated in memory was sent.
func (c *Context) AuthorizePendingReset(ctx context.Context, version uint16) error {
	if err := c.enter(ctx); err != nil {
		return err
	}
	defer c.mu.Unlock()
	c.clearPendingReset()
	if !c.initialized || c.established || version == 0 || version != c.version ||
		c.generation == 0 || c.keyPackageGeneration != c.generation {
		return ErrNotReady
	}
	c.pendingResetGeneration = c.generation
	c.pendingResetVersion = version
	return nil
}
