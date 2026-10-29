# Native DAVE boundary

This package is compiled only with `voice && cgo`. It owns ephemeral MLS, sender,
receiver and prepared-transition handles; it contains no Gateway, RTP, UDP, job,
media decoder, persisted identity, or replacement cryptographic implementation.

## Exact native input

- Upstream: official `discord/libdave` release `v1.2.2/cpp`.
- Source commit: `6396c90aedbb9a6ee117ea9ce1ce9832af6a3375`.
- Required project source patch: `tools/native/libdave.patch`.
- Patch SHA-256: `4f315a8af3f4b464b846644aeca5054010493241762f8f2ffc85695eb3016511`.
- This is a **source-patched fork**, not an unmodified upstream release artifact.

Pending-self export is a project integration inference from the public protocol
and pinned native state model, not a documented upstream activation API or a
claim about recovered proprietary Discord-client behavior. Interoperability
acceptance remains an explicit integration gate.

The patch adds `dave/flex_dave_ext.h` and two C symbols:

```c
bool flexDaveSessionSetProposalLimits(DAVESessionHandle, size_t, size_t);
bool flexDaveSessionActivatePendingGroup(DAVESessionHandle);
```

The first configures mandatory positive queued-proposal count and serialized-byte
limits in the official parser/queue path. Its outer list is decoded once using
MLSPP's TLS parser, each parsed message is validated and processed normally, and
queue entries retain their exact wire-byte charge. Revoke subtracts the removed
entry's charge; rejected batches discard partial proposal caches and stale
outbound candidates, without discarding the current/pending group or latest join
package. Empty queues generate no synthetic commit. Reset clears the old roster
so the first new-group change map cannot omit a member whose signature key was
retained across the reset.

The second authorizes **pending-self media export only** after a sanctioned
immediate transition zero. It checks supported nonzero version, pending epoch
zero, exactly one basic self credential with the expected signature public key,
expected group/ciphersuite, exactly the expected external sender, and a fresh
join package/private-key pair. It does **not** promote pending state into
`currentState_`, choose a commit, consume the join package, or change the official
exporter, ratchet derivation, cipher, or wire format. Reset/recreation clears its
flag. The Go owner additionally requires current-generation package production,
the recognized participant set to contain only self, and an explicit
`AuthorizePendingReset(ctx, version)` stamp. The Gateway owner may grant that
stamp only after an authenticated opcode-24 epoch-one reset and successful
opcode-26 write for the matching generation/version. Initial Session Description
or invalid-commit recovery alone cannot authorize pending-self activation.
The matching pending opcode-21 transition zero consumes the stamp. Reset, fresh
package replacement, `SetProtocolVersion` (including unchanged versions), control
failure and genuine establishment invalidate it. A duplicate already-active
transition zero is an idempotent no-op, never a ratchet/nonce reinstall. The fresh
join package and private key remain native for a later genuine Commit/Welcome.

An external-sender package can arrive after that generation's fresh key package
was sent. Pinned `SetExternalSender`/`CreatePendingGroup` preserve the generated
join package, private join key and local leaf. Installing the public sender
clears any native-owner authorization stamp, but the Gateway owner can defer or
refresh authorization after successful opcode 25 using its still-unconsumed
matching authenticated-reset/successful-write record. A generated package or a
new external sender alone never creates that record.

## Headers, link and build responsibilities

CGO needs the include root containing both `dave/dave.h` from the pinned source
and the installed patched `dave/flex_dave_ext.h`. It compiles the C callback bridge
as C11 and links `-ldave`. Supply include/library search paths through the native
builder's `CGO_CFLAGS`/`CGO_LDFLAGS` or installed toolchain configuration. No MLSPP
C++ headers are needed by CGO; they are needed by the separate native source
build. Source-build requirements and dependency pins remain the proposal's C++17,
CMake >= 3.20, pinned MLSPP/vcpkg and matching BoringSSL closure. Do not mix crypto
closures or load an ABI-incompatible prebuilt library.

Native download/digest/header-symbol/loader checks, target-compatible source
builds, native license/runtime closure, formatter, builds, tests and live Discord
acceptance are integration-owner responsibilities. Source implementation and
fixtures alone are not evidence of playable or received voice.

## Handle and byte ownership audit

| Native resource | Owner and release |
| --- | --- |
| Session | One `Context`; `daveSessionDestroy` before callback state free. |
| Callback state | C `malloc`-owned atomic flag; C `free` after all admitted calls and native session destruction. No Go callback/pointer is retained. |
| Encryptor | One `Context`; no passthrough; `daveEncryptorDestroy` on close. |
| Prepared self/receiver ratchets | `daveSessionGetKeyRatchet` transfers each new opaque owner to the prepared transition. Destroy after native media installation or discard/close. |
| Installed encrypt/decrypt ratchets | Pinned `bindings_capi.cpp` **copies** borrowed input ratchets through `CopyKeyRatchet`. Destroying the original is correct, not a transferred-owner double free. |
| Decryptors | Recognized sender-specific `Context` map, bounded by configured count; native `TransitionToKeyRatchet` owns its copies and expires old domains using its official ten-second window. |
| Commit/Welcome result | Destroy once after consuming bounded roster deltas. Removed members have an empty signature; this is not a complete roster snapshot. |
| Native returned bytes, roster IDs and signatures | `daveFree`, including rejected oversized outputs. Check size before Go copy; signatures are inspected for removal and freed without Go copying. |
| Frame buffers | Borrowed caller buffers for the duration of a serialized synchronous call only; native maximum-size queries precede writes; no per-frame Go allocation/copy. Failed output is cleared. |

A context lock serializes all native MLS/media operations and close. The only
registered MLS callback is synchronous in the pinned source and writes a C-owned
atomic failure flag, never source/reason text. Asynchronous fingerprint callbacks
are not used. Acquiring the close lock joins admitted calls and their callbacks;
callback memory is freed last. A global C-only discard sink prevents upstream
verbose MLS bytes, key-domain addresses and raw error text from entering logs.
No callback sink memory is freed while libdave could reference it.

Prepared contexts and serialized transition/control memory have explicit finite
bounds. Participant/decryptor counts bound opaque native state; the receiver
ledger also bounds simultaneous retained native key domains, including each
active domain and old domains within the official ten-second window. Sender
changes occur only at Execute Transition or immediate transition zero. A reset
preserves the valid old sender until that boundary. Same-domain transitions do
not reset the encryptor nonce sequence. Unknown/unrecognized senders and failed
native authentication never yield plaintext. The official unauthenticated SFU
silence exception is discarded by this strict authenticated-Opus API.

## API and unexecuted fixtures

`New(Config)` has no defaults; `Config.Validate` checks every required finite
bound. The proposal's context-first methods are implemented, with additive
`SetParticipants`, `SetProtocolVersion`, `AuthorizePendingReset`,
`PrepareTransition`, `Ready`, `MaxCiphertextSize`, and `MaxPlaintextSize` for the
transport owner. Proposal input excludes its append/revoke byte;
`CommitWelcome.Wire` is the official opaque combined opcode-28 payload. On a
successful call its emptiness is exactly native proposal-queue emptiness; a
nonempty queue synchronously produces its native candidate, while failures
return an error rather than successful empty output. Applied, ignored and failed
commits remain distinct; an ignored result is not a recovery-triggering callback
error and does not revoke a valid pending reset.

`context_test.go` exercises real native fresh packages, pending-only immediate
activation, non-sole rejection, native Opus encryption, sole reset preserving the
old sender until execution, unavailable/plaintext/size boundaries, ignored versus
failed commits, malformed proposal framing, no empty-proposal synthetic commit,
and close racing the native failure callback. `authorization_test.go`
additionally covers generated-only/stale/superseded reset authority, admitted
control-error revocation and ignored-versus-failed semantics. These fixtures
require the patched native library; they have not been run by the source owner.
Integration must also
exercise genuine multi-member Commit/Welcome and receive authentication/replay,
old-epoch in-flight media, actual proposal-count/byte/revoke limits, and an
approved Discord sole-reset-to-later-member-join flow.
