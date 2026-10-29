//go:build voice && cgo

// Package dave owns ephemeral libdave MLS and media contexts. It is deliberately
// absent from ordinary builds; it never implements media cryptography itself.
package dave

/*
#cgo CFLAGS: -std=c11
#cgo LDFLAGS: -ldave
#include "bridge.h"
*/
import "C"

import (
	"context"
	"errors"
	"runtime"
	"strconv"
	"sync"
	"time"
	"unsafe"
)

// Config has no defaults. Limits apply before native input allocations and
// before copying native output into Go. Frame buffers remain caller-owned.
type Config struct {
	MaxParticipants        int
	MaxDecryptors          int
	MaxControlBytes        int
	MaxProposalCount       int
	MaxProposalBytes       int
	MaxPreparedTransitions int
	MaxTransitionBytes     int
	MaxFrameBytes          int
	MaxEncryptedFrameBytes int
}

var (
	ErrConfig         = errors.New("dave: invalid finite bounds")
	ErrClosed         = errors.New("dave: context closed")
	ErrBounds         = errors.New("dave: resource bound exceeded")
	ErrInput          = errors.New("dave: invalid control input")
	ErrNative         = errors.New("dave: native operation failed")
	ErrNotReady       = errors.New("dave: media context not ready")
	ErrUnsupported    = errors.New("dave: unsupported protocol version")
	ErrUnknownSender  = errors.New("dave: sender not recognized")
	ErrBuffer         = errors.New("dave: insufficient output buffer")
	ErrTransition     = errors.New("dave: invalid transition")
	ErrAuthentication = errors.New("dave: frame authentication failed")
)

type ProposalOperation uint8

const (
	ProposalAppend ProposalOperation = iota
	ProposalRevoke
)

// Wire is the official ABI's serialized commit followed by its optional welcome.
// It is one opaque opcode-28 payload, not two independently framed messages.
type CommitWelcome struct{ Wire []byte }

type CommitResult uint8

const (
	CommitApplied CommitResult = iota
	CommitIgnored
	CommitFailed
)

// The C ABI's TransitionToKeyRatchet uses kDefaultTransitionDuration=10s. The
// admission ledger here bounds retained native key domains within that window;
// native decryptors, not Go, authenticate against and expire the old ratchets.
const receiverTransitionWindow = 10 * time.Second

type receiver struct {
	handle     C.DAVEDecryptorHandle
	epochs     []time.Time
	generation uint64
	retired    time.Time
}

type transition struct {
	version    uint16
	sender     C.DAVEKeyRatchetHandle
	receivers  map[string]C.DAVEKeyRatchetHandle
	prepared   bool
	bytes      int
	generation uint64
}

// Context serializes all MLS, frame, callback-state and close operations. No
// native callback invokes Go, and no Go pointer is retained by libdave.
type Context struct {
	mu                     sync.Mutex
	cfg                    Config
	session                C.DAVESessionHandle
	failure                *C.flex_dave_failure
	encryptor              C.DAVEEncryptorHandle
	closed                 bool
	initialized            bool
	established            bool
	version                uint16
	self                   string
	participants           map[string]struct{}
	roster                 map[string]struct{}
	receivers              map[string]*receiver
	transitions            map[uint16]*transition
	transitionBytes        int
	proposalScratch        []byte
	active                 bool
	activeID               uint16
	generation             uint64
	receivingGeneration    uint64
	activeGeneration       uint64
	keyPackageGeneration   uint64
	pendingResetGeneration uint64
	pendingResetVersion    uint16
	ssrc                   uint32
	ssrcBound              bool
}

var suppressLogs sync.Once

func MaxSupportedProtocolVersion() uint16 {
	// This adapter supports the currently pinned protocol, not an unreviewed
	// future native release's additional protocols.
	v := uint16(C.daveMaxSupportedProtocolVersion())
	if v > 1 {
		return 1
	}
	return v
}

func (cfg Config) Validate() error {
	for _, n := range []int{cfg.MaxParticipants, cfg.MaxDecryptors,
		cfg.MaxControlBytes, cfg.MaxProposalCount, cfg.MaxProposalBytes,
		cfg.MaxPreparedTransitions, cfg.MaxTransitionBytes, cfg.MaxFrameBytes,
		cfg.MaxEncryptedFrameBytes} {
		if n <= 0 {
			return ErrConfig
		}
	}
	// Native control copies use Go's bounded length; frame-size arithmetic must
	// leave room for the library's transform overhead without uintptr overflow.
	maxInt := int(^uint(0) >> 1)
	if cfg.MaxControlBytes > maxInt-1 || cfg.MaxFrameBytes > maxInt-cfg.MaxEncryptedFrameBytes ||
		cfg.MaxFrameBytes > cfg.MaxEncryptedFrameBytes {
		return ErrConfig
	}
	return nil
}

func New(cfg Config) (*Context, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	suppressLogs.Do(func() { C.flex_dave_suppress_logs() })
	c := &Context{cfg: cfg, participants: make(map[string]struct{}),
		roster: make(map[string]struct{}), receivers: make(map[string]*receiver),
		transitions: make(map[uint16]*transition)}
	c.failure = C.flex_dave_failure_create()
	if c.failure == nil {
		return nil, ErrNative
	}
	c.session = C.flex_dave_session_create(c.failure)
	c.encryptor = C.daveEncryptorCreate()
	if c.session == nil || c.encryptor == nil {
		c.Close()
		return nil, ErrNative
	}
	if !bool(C.flexDaveSessionSetProposalLimits(c.session, C.size_t(cfg.MaxProposalCount), C.size_t(cfg.MaxProposalBytes))) {
		c.Close()
		return nil, ErrConfig
	}
	C.daveEncryptorSetPassthroughMode(c.encryptor, C.bool(false))
	n := C.daveEncryptorGetMaxCiphertextByteSize(c.encryptor, C.DAVE_MEDIA_TYPE_AUDIO, C.size_t(cfg.MaxFrameBytes))
	if n > C.size_t(cfg.MaxEncryptedFrameBytes) || n < C.size_t(cfg.MaxFrameBytes) {
		c.Close()
		return nil, ErrConfig
	}
	return c, nil
}

func (c *Context) enter(ctx context.Context) error {
	if ctx == nil {
		return ErrInput
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return ErrClosed
	}
	if err := ctx.Err(); err != nil {
		c.mu.Unlock()
		return err
	}
	return nil
}

func validID(id string) bool {
	if len(id) == 0 || len(id) > 20 {
		return false
	}
	n, err := strconv.ParseUint(id, 10, 64)
	return err == nil && n != 0 && strconv.FormatUint(n, 10) == id
}

func pointer(b []byte) *C.uint8_t { return (*C.uint8_t)(unsafe.Pointer(unsafe.SliceData(b))) }

func (c *Context) checkNative() error {
	if bool(C.flex_dave_failure_failed(c.failure)) {
		return ErrNative
	}
	return nil
}

func (c *Context) resetFailure() { C.flex_dave_failure_reset(c.failure) }

func (c *Context) Initialize(ctx context.Context, version uint16, groupID, selfUserID string) error {
	if err := c.enter(ctx); err != nil {
		return err
	}
	defer c.mu.Unlock()
	c.clearPendingReset()
	if version == 0 || version > MaxSupportedProtocolVersion() {
		return ErrUnsupported
	}
	if !validID(groupID) || !validID(selfUserID) {
		return ErrInput
	}
	if c.initialized && c.self != selfUserID {
		return ErrInput
	}
	if c.generation == ^uint64(0) {
		return ErrBounds
	}
	group, _ := strconv.ParseUint(groupID, 10, 64)
	self := C.CString(selfUserID)
	if self == nil {
		return ErrNative
	}
	defer C.free(unsafe.Pointer(self))
	c.resetFailure()
	C.daveSessionInit(c.session, C.uint16_t(version), C.uint64_t(group), self)
	if err := c.checkNative(); err != nil {
		return err
	}
	for id, t := range c.transitions {
		destroyTransition(t)
		delete(c.transitions, id)
	}
	c.transitionBytes = 0
	c.version, c.self, c.initialized = version, selfUserID, true
	c.established = false
	c.generation++
	c.keyPackageGeneration = 0
	// Reset only MLS/pending state. Existing native media contexts remain valid
	// until the gateway executes the new transition.
	c.roster = map[string]struct{}{selfUserID: {}}
	return nil
}

func (c *Context) SetProtocolVersion(ctx context.Context, version uint16) (resultErr error) {
	if err := c.enter(ctx); err != nil {
		return err
	}
	defer c.mu.Unlock()
	c.clearPendingReset()
	if !c.initialized {
		return ErrNotReady
	}
	if version == 0 || version > MaxSupportedProtocolVersion() {
		return ErrUnsupported
	}
	C.daveSessionSetProtocolVersion(c.session, C.uint16_t(version))
	c.version = version
	return nil
}

func (c *Context) SetExternalSender(ctx context.Context, senderPackage []byte) (resultErr error) {
	if err := c.enter(ctx); err != nil {
		return err
	}
	defer c.mu.Unlock()
	c.clearPendingReset()
	if len(senderPackage) == 0 || len(senderPackage) > c.cfg.MaxControlBytes {
		return ErrBounds
	}
	// Replacing a pending group's sender after its media domain was activated
	// would recreate that group within the same generation. Require a real reset.
	if c.initialized && !c.established && c.activeGeneration == c.generation {
		return ErrTransition
	}
	c.resetFailure()
	C.daveSessionSetExternalSender(c.session, pointer(senderPackage), C.size_t(len(senderPackage)))
	runtime.KeepAlive(senderPackage)
	return c.checkNative()
}

func (c *Context) copyOutput(p *C.uint8_t, n C.size_t, required bool) ([]byte, error) {
	if p != nil {
		defer C.daveFree(unsafe.Pointer(p))
	}
	if n > C.size_t(c.cfg.MaxControlBytes) {
		return nil, ErrBounds
	}
	if n == 0 {
		if required {
			return nil, ErrNative
		}
		return nil, nil
	}
	if p == nil {
		return nil, ErrNative
	}
	out := make([]byte, int(n))
	copy(out, unsafe.Slice((*byte)(unsafe.Pointer(p)), int(n)))
	return out, nil
}

func (c *Context) FreshKeyPackage(ctx context.Context) ([]byte, error) {
	if err := c.enter(ctx); err != nil {
		return nil, err
	}
	defer c.mu.Unlock()
	c.clearPendingReset()
	if !c.initialized {
		return nil, ErrNotReady
	}
	// Native replaces the previous private join package even if output fails
	// our copy bound; an older package must not authorize pending activation.
	c.keyPackageGeneration = 0
	var p *C.uint8_t
	var n C.size_t
	c.resetFailure()
	C.daveSessionGetMarshalledKeyPackage(c.session, &p, &n)
	if err := c.checkNative(); err != nil {
		if p != nil {
			C.daveFree(unsafe.Pointer(p))
		}
		return nil, err
	}
	out, err := c.copyOutput(p, n, true)
	if err == nil {
		c.keyPackageGeneration = c.generation
	}
	return out, err
}

func (c *Context) setParticipants(ids []string) error {
	if len(ids) > c.cfg.MaxParticipants {
		return ErrBounds
	}
	next := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if !validID(id) {
			return ErrInput
		}
		if _, exists := next[id]; exists {
			return ErrInput
		}
		next[id] = struct{}{}
	}
	c.participants = next
	if c.self != "" {
		if _, selfPresent := next[c.self]; !selfPresent {
			c.clearPendingReset()
		}
	}
	for id, r := range c.receivers {
		if _, exists := next[id]; !exists {
			C.daveDecryptorDestroy(r.handle)
			delete(c.receivers, id)
		}
	}
	return nil
}

func (c *Context) SetParticipants(ctx context.Context, ids []string) (resultErr error) {
	if err := c.enter(ctx); err != nil {
		return err
	}
	defer c.mu.Unlock()
	defer c.clearPendingResetOnError(&resultErr)
	return c.setParticipants(ids)
}

// recognizedArray returns C-owned pointer storage AND C-owned strings. Native
// constructors copy them synchronously; none is retained after this call.
func recognizedArray(ids []string) (array **C.char, release func(), err error) {
	count := len(ids)
	if count == 0 {
		// The official C++ binding constructs a range even for an empty set.
		// Supply a valid zero-length range rather than nullptr arithmetic.
		count = 1
	}
	mem := C.calloc(C.size_t(count), C.size_t(unsafe.Sizeof(uintptr(0))))
	if mem == nil {
		return nil, nil, ErrNative
	}
	array = (**C.char)(mem)
	slots := unsafe.Slice(array, count)
	release = func() {
		for _, p := range slots {
			if p != nil {
				C.free(unsafe.Pointer(p))
			}
		}
		C.free(mem)
	}
	for i, id := range ids {
		slots[i] = C.CString(id)
		if slots[i] == nil {
			release()
			return nil, nil, ErrNative
		}
	}
	return array, release, nil
}

func destroyTransition(t *transition) {
	if t.sender != nil {
		C.daveKeyRatchetDestroy(t.sender)
		t.sender = nil
	}
	for id, r := range t.receivers {
		C.daveKeyRatchetDestroy(r)
		delete(t.receivers, id)
	}
}

func (c *Context) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	c.clearPendingReset()
	// Lock acquisition joins all admitted operations and their synchronous MLS
	// callbacks. No fingerprint/asynchronous callbacks are ever registered.
	for id, t := range c.transitions {
		destroyTransition(t)
		delete(c.transitions, id)
	}
	for id, r := range c.receivers {
		C.daveDecryptorDestroy(r.handle)
		delete(c.receivers, id)
	}
	if c.encryptor != nil {
		C.daveEncryptorDestroy(c.encryptor)
		c.encryptor = nil
	}
	if c.session != nil {
		C.daveSessionDestroy(c.session)
		c.session = nil
	}
	// Destroy callback memory only after the native session can no longer call it.
	if c.failure != nil {
		C.flex_dave_failure_destroy(c.failure)
		c.failure = nil
	}
	c.participants, c.roster = nil, nil
	c.self, c.version, c.active = "", 0, false
	c.transitionBytes = 0
	clear(c.proposalScratch)
	c.proposalScratch = nil
	return nil
}
