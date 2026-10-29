//go:build voice && cgo

package dave

/*
#include "bridge.h"
*/
import "C"

import (
	"context"
	"runtime"
	"strconv"
	"time"
	"unsafe"
)

func (c *Context) transitionAdmission(id uint16, bytes int) error {
	if _, exists := c.transitions[id]; exists {
		return ErrTransition
	}
	if len(c.transitions) >= c.cfg.MaxPreparedTransitions || bytes < 0 ||
		bytes > c.cfg.MaxTransitionBytes-c.transitionBytes {
		return ErrBounds
	}
	return nil
}

// applyRoster consumes the official CHANGES map, not a full-membership snapshot.
// Empty member signatures indicate removals. Signatures stay native and are
// freed without copying or exposing them outside this ownership boundary.
func (c *Context) applyRoster(ids *C.uint64_t, count C.size_t, signature func(C.uint64_t, **C.uint8_t, *C.size_t)) error {
	if ids != nil {
		defer C.daveFree(unsafe.Pointer(ids))
	}
	// A change set can contain both all old removals and all new additions.
	if count > C.size_t(c.cfg.MaxControlBytes/8) ||
		uint64(count) > uint64(c.cfg.MaxParticipants)*2 {
		return ErrBounds
	}
	if count != 0 && ids == nil {
		return ErrNative
	}
	next := make(map[string]struct{}, len(c.roster))
	for id := range c.roster {
		next[id] = struct{}{}
	}
	for _, rawID := range unsafe.Slice(ids, int(count)) {
		id := strconv.FormatUint(uint64(rawID), 10)
		if !validID(id) {
			return ErrNative
		}
		var p *C.uint8_t
		var n C.size_t
		signature(rawID, &p, &n)
		if n > C.size_t(c.cfg.MaxControlBytes) || (n != 0 && p == nil) {
			if p != nil {
				C.daveFree(unsafe.Pointer(p))
			}
			return ErrBounds
		}
		if p != nil {
			C.daveFree(unsafe.Pointer(p))
		}
		if n == 0 {
			delete(next, id)
			continue
		}
		if id != c.self {
			if _, recognized := c.participants[id]; !recognized {
				return ErrUnknownSender
			}
		}
		next[id] = struct{}{}
	}
	if len(next) > c.cfg.MaxParticipants {
		return ErrBounds
	}
	if _, exists := next[c.self]; !exists {
		return ErrNative
	}
	c.roster = next
	return nil
}

func (c *Context) ratchet(id string) (C.DAVEKeyRatchetHandle, error) {
	user := C.CString(id)
	if user == nil {
		return nil, ErrNative
	}
	defer C.free(unsafe.Pointer(user))
	h := C.daveSessionGetKeyRatchet(c.session, user)
	if h == nil {
		return nil, ErrNotReady
	}
	return h, nil
}

func (c *Context) snapshotTransition(id uint16, bytes int) error {
	if err := c.transitionAdmission(id, bytes); err != nil {
		return err
	}
	t := &transition{version: c.version, bytes: bytes, generation: c.generation,
		receivers: make(map[string]C.DAVEKeyRatchetHandle)}
	sender, err := c.ratchet(c.self)
	if err != nil {
		return err
	}
	t.sender = sender
	for user := range c.roster {
		if user == c.self {
			continue
		}
		if _, recognized := c.participants[user]; !recognized {
			continue
		}
		if len(t.receivers) >= c.cfg.MaxDecryptors {
			destroyTransition(t)
			return ErrBounds
		}
		h, err := c.ratchet(user)
		if err != nil {
			destroyTransition(t)
			return err
		}
		t.receivers[user] = h
	}
	c.transitions[id] = t
	c.transitionBytes += bytes
	return nil
}

func (c *Context) ProcessCommit(ctx context.Context, transitionID uint16, commit []byte) (outcome CommitResult, resultErr error) {
	if err := c.enter(ctx); err != nil {
		return CommitFailed, err
	}
	defer c.mu.Unlock()
	defer c.clearPendingResetOnError(&resultErr)
	if !c.initialized {
		return CommitFailed, ErrNotReady
	}
	if len(commit) == 0 || len(commit) > c.cfg.MaxControlBytes {
		return CommitFailed, ErrBounds
	}
	if err := c.transitionAdmission(transitionID, len(commit)); err != nil {
		return CommitFailed, err
	}
	if c.generation == ^uint64(0) {
		return CommitFailed, ErrBounds
	}
	c.resetFailure()
	result := C.daveSessionProcessCommit(c.session, pointer(commit), C.size_t(len(commit)))
	runtime.KeepAlive(commit)
	if result == nil {
		return CommitFailed, ErrNative
	}
	defer C.daveCommitResultDestroy(result)
	// The upstream ignored branch can invoke the failure callback. Check the
	// semantic result first; ignored is explicitly not invalid-commit recovery.
	if bool(C.daveCommitResultIsIgnored(result)) {
		return CommitIgnored, nil
	}
	if bool(C.daveCommitResultIsFailed(result)) {
		return CommitFailed, ErrNative
	}
	if err := c.checkNative(); err != nil {
		return CommitFailed, err
	}
	var ids *C.uint64_t
	var count C.size_t
	C.daveCommitResultGetRosterMemberIds(result, &ids, &count)
	if err := c.applyRoster(ids, count, func(id C.uint64_t, p **C.uint8_t, n *C.size_t) {
		C.daveCommitResultGetRosterMemberSignature(result, id, p, n)
	}); err != nil {
		return CommitFailed, err
	}
	c.established = true
	c.clearPendingReset()
	c.generation++
	if err := c.snapshotTransition(transitionID, len(commit)); err != nil {
		return CommitFailed, err
	}
	return CommitApplied, nil
}

func (c *Context) ProcessWelcome(ctx context.Context, transitionID uint16, welcome []byte, recognizedIDs []string) (resultErr error) {
	if err := c.enter(ctx); err != nil {
		return err
	}
	defer c.mu.Unlock()
	defer c.clearPendingResetOnError(&resultErr)
	if !c.initialized {
		return ErrNotReady
	}
	if len(welcome) == 0 || len(welcome) > c.cfg.MaxControlBytes {
		return ErrBounds
	}
	if err := c.transitionAdmission(transitionID, len(welcome)); err != nil {
		return err
	}
	if c.generation == ^uint64(0) {
		return ErrBounds
	}
	if err := c.setParticipants(recognizedIDs); err != nil {
		return err
	}
	recognized, release, err := recognizedArray(recognizedIDs)
	if err != nil {
		return err
	}
	defer release()
	c.resetFailure()
	result := C.daveSessionProcessWelcome(c.session, pointer(welcome), C.size_t(len(welcome)),
		recognized, C.size_t(len(recognizedIDs)))
	runtime.KeepAlive(welcome)
	if result == nil {
		return ErrNative
	}
	defer C.daveWelcomeResultDestroy(result)
	if err := c.checkNative(); err != nil {
		return err
	}
	var ids *C.uint64_t
	var count C.size_t
	C.daveWelcomeResultGetRosterMemberIds(result, &ids, &count)
	if err := c.applyRoster(ids, count, func(id C.uint64_t, p **C.uint8_t, n *C.size_t) {
		C.daveWelcomeResultGetRosterMemberSignature(result, id, p, n)
	}); err != nil {
		return err
	}
	c.established = true
	c.clearPendingReset()
	c.generation++
	return c.snapshotTransition(transitionID, len(welcome))
}

func (c *Context) expireReceivers(now time.Time) {
	for user, r := range c.receivers {
		if !r.retired.IsZero() && !r.retired.After(now) {
			C.daveDecryptorDestroy(r.handle)
			delete(c.receivers, user)
		}
	}
}

func (c *Context) prepareReceivers(id uint16) error {
	t := c.transitions[id]
	if t == nil || t.generation < c.receivingGeneration {
		return ErrTransition
	}
	if t.prepared {
		return nil
	}
	now := time.Now()
	c.expireReceivers(now)
	additional := 0
	for user := range t.receivers {
		if _, recognized := c.participants[user]; !recognized {
			continue
		}
		if r := c.receivers[user]; r != nil {
			if r.generation == t.generation && r.retired.IsZero() {
				continue
			}
			first := 0
			for first < len(r.epochs) && !r.epochs[first].IsZero() && !r.epochs[first].After(now) {
				first++
			}
			copy(r.epochs, r.epochs[first:])
			r.epochs = r.epochs[:len(r.epochs)-first]
			if len(r.epochs) >= c.cfg.MaxPreparedTransitions {
				return ErrBounds
			}
		} else {
			additional++
		}
	}
	if additional > c.cfg.MaxDecryptors-len(c.receivers) {
		return ErrBounds
	}
	// Allocate all required decryptors before mutating any existing ratchets.
	created := make(map[string]*receiver, additional)
	for user := range t.receivers {
		if _, recognized := c.participants[user]; !recognized {
			continue
		}
		if c.receivers[user] != nil {
			continue
		}
		h := C.daveDecryptorCreate()
		if h == nil {
			for _, r := range created {
				C.daveDecryptorDestroy(r.handle)
			}
			return ErrNative
		}
		// Native default is no passthrough. Never turn it on.
		created[user] = &receiver{handle: h}
	}
	for user, r := range created {
		c.receivers[user] = r
	}
	for user, r := range c.receivers {
		if _, member := t.receivers[user]; member || !r.retired.IsZero() {
			continue
		}
		// Removal expires the former active domain using the same official
		// ten-second transition facility, without admitting plaintext.
		C.daveDecryptorTransitionToKeyRatchet(r.handle, nil)
		r.retired = now.Add(receiverTransitionWindow)
		for i := range r.epochs {
			if r.epochs[i].IsZero() {
				r.epochs[i] = r.retired
			}
		}
	}
	for user, ratchet := range t.receivers {
		if _, recognized := c.participants[user]; recognized {
			r := c.receivers[user]
			if r.generation != t.generation || !r.retired.IsZero() {
				// The C ABI copies the key domain; input remains ours.
				C.daveDecryptorTransitionToKeyRatchet(r.handle, ratchet)
				for i := range r.epochs {
					if r.epochs[i].IsZero() {
						r.epochs[i] = now.Add(receiverTransitionWindow)
					}
				}
				r.epochs = append(r.epochs, time.Time{})
				r.generation, r.retired = t.generation, time.Time{}
			}
		}
		C.daveKeyRatchetDestroy(ratchet)
		delete(t.receivers, user)
	}
	c.receivingGeneration = t.generation
	t.prepared = true
	return nil
}

func (c *Context) PrepareReceivers(ctx context.Context, transitionID uint16) (resultErr error) {
	if err := c.enter(ctx); err != nil {
		return err
	}
	defer c.mu.Unlock()
	defer c.clearPendingResetOnError(&resultErr)
	if err := c.prepareReceivers(transitionID); err != nil {
		return err
	}
	if transitionID == 0 {
		return c.executeTransition(transitionID)
	}
	return nil
}

// PrepareTransition handles opcode 21, including immediate transition zero. A
// commit/welcome transition has already snapshotted its own native key domains.
func (c *Context) PrepareTransition(ctx context.Context, transitionID, version uint16) (resultErr error) {
	if err := c.enter(ctx); err != nil {
		return err
	}
	defer c.mu.Unlock()
	defer c.clearPendingResetOnError(&resultErr)
	if !c.initialized {
		return ErrNotReady
	}
	if version == 0 || version > MaxSupportedProtocolVersion() {
		return ErrUnsupported
	}
	if transitionID != 0 {
		c.clearPendingReset()
	}
	if t := c.transitions[transitionID]; t != nil {
		if t.version != version {
			return ErrTransition
		}
	} else {
		C.daveSessionSetProtocolVersion(c.session, C.uint16_t(version))
		c.version = version
		if transitionID == 0 && !c.established {
			if c.active && c.activeGeneration == c.generation && c.activeID == 0 {
				c.clearPendingReset()
				return nil
			}
			if c.pendingResetGeneration != c.generation || c.pendingResetVersion != version {
				return ErrNotReady
			}
			if c.keyPackageGeneration != c.generation || len(c.participants) != 1 {
				return ErrNotReady
			}
			if _, soleSelf := c.participants[c.self]; !soleSelf {
				return ErrNotReady
			}
			c.clearPendingReset()
			if !bool(C.flexDaveSessionActivatePendingGroup(c.session)) {
				return ErrNotReady
			}
		}
		if err := c.snapshotTransition(transitionID, 0); err != nil {
			return err
		}
	}
	if err := c.prepareReceivers(transitionID); err != nil {
		return err
	}
	if transitionID == 0 {
		return c.executeTransition(transitionID)
	}
	return nil
}

func (c *Context) executeTransition(id uint16) error {
	t := c.transitions[id]
	if t == nil {
		if c.active && c.activeID == id {
			return nil
		}
		return ErrTransition
	}
	if !t.prepared || t.sender == nil {
		return ErrNotReady
	}
	if t.generation < c.activeGeneration {
		return ErrTransition
	}
	if t.generation != c.activeGeneration {
		C.daveEncryptorSetKeyRatchet(c.encryptor, t.sender)
	}
	C.daveKeyRatchetDestroy(t.sender)
	t.sender = nil
	if !bool(C.daveEncryptorHasKeyRatchet(c.encryptor)) || bool(C.daveEncryptorIsPassthroughMode(c.encryptor)) {
		return ErrNative
	}
	c.active, c.activeID = true, id
	c.activeGeneration = t.generation
	c.transitionBytes -= t.bytes
	destroyTransition(t)
	delete(c.transitions, id)
	// An executed newer domain makes older prepared sender contexts obsolete.
	// Receiver in-flight retention remains in the official decryptors.
	for oldID, old := range c.transitions {
		if old.generation < c.activeGeneration {
			c.transitionBytes -= old.bytes
			destroyTransition(old)
			delete(c.transitions, oldID)
		}
	}
	return nil
}

func (c *Context) ExecuteTransition(ctx context.Context, transitionID uint16) (resultErr error) {
	if err := c.enter(ctx); err != nil {
		return err
	}
	defer c.mu.Unlock()
	defer c.clearPendingResetOnError(&resultErr)
	return c.executeTransition(transitionID)
}

func (c *Context) Ready() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.closed && c.active && bool(C.daveEncryptorHasKeyRatchet(c.encryptor)) &&
		!bool(C.daveEncryptorIsPassthroughMode(c.encryptor))
}
