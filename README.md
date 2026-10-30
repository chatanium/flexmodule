# FlexModule

**Next Generation Module System for Chatanium**

FlexModule is a generic, backend-neutral module runtime for Chatanium. The current
entry point uses the v2 protocol: independently installed modules run in
Docker containers with runsc/gVisor, communicate over authenticated bidirectional
gRPC, and receive only explicitly approved capabilities and resource scopes.
Discord is an optional backend. Music and voice are optional module/backend
features, not prerequisites for an ordinary management or message bot.

## Architecture

![FlexModule Diagram](https://github.com/user-attachments/assets/1cf77e2c-ea04-47be-b2c5-1d4bb7bd9fc6)

This original diagram records the project's multi-provider direction and intended
integration into Chatanium v2. It is historical design context, not the current
process or security topology:

```text
Operator-approved configuration and immutable OCI images
    |
    +-- privileged flexmodule-supervisor (independent host service)
    |     Docker/runsc, quota, network policy, cleanup/reconciliation
    |
    +-- unprivileged runtime
          core manager, ordered event admission, uploads, credentials
          optional Discord backend (host-only bot token and Gateway)
          optional voice service (separate native-enabled artifact)
              |
              +-- runsc module instances
                    authenticated core-v2 Hook
                    one fixed reverse connection for core/file/backend helpers
                    authenticated storage paths, explicit resource/egress approval
```

Modules must use the authenticated `InitContext.persistent_directory` and
`InitContext.scratch_directory` values, never hardcode mount paths. The current
host constants are `/var/lib/flexmodule/data` and
`/var/lib/flexmodule/scratch`; these implementation locations are not substitutes
for the paths supplied during initialization.

The runtime does not use `PLUGIN_PATH`, launch module binaries directly on the
host, or fall back to runc. go-plugin remains the authenticated AutoMTLS/multiplex
transport inside the sandbox runner; it is not the security boundary by itself.
Each instance permits one reverse-helper transport for its entire lifetime,
using fixed broker ID 1. It is not one connection per helper or backend, and
reconnection/replacement is not authorized. Helper transport failure fails the
instance rather than silently reconnecting with old authority.
Shutdown closes and joins the primary physical yamux session, not just its gRPC
stream, before releasing pinned socket descriptors and requesting host cleanup.
Temporary storage claims remain reserved until the paths are removed and XFS
allocation counters are zero. Cleanup synchronizes once, then observes the XFS
reclamation barrier and exact project counters within the existing cleanup
deadline; deferred inode reclamation is not mistaken for an immediate leak.
Kernel errors or an expired deadline remain cleanup failures and retain claims,
never a successful stop.

Discord helper HTTP requests use fresh HTTP/1 connections with matching
`http/1.1` TLS ALPN, not the default Go transport's inherited `h2` advertisement.
Redirects and automatic mutation replays remain disabled.

Current entry points:

| Path | Responsibility |
|---|---|
| `cmd/flexmodule/` | Runtime CLI, configuration generation and optional backend composition |
| `cmd/flexmodule-supervisor/`, `internal/host/` | Privileged host control and Docker/runsc execution |
| `internal/runtime/`, `internal/runtime/upload/` | Backend-neutral instance lifecycle, grants, admission and file ownership |
| `proto/core-v2/`, `shared/core-v2/` | Lifecycle, configuration, file transfer and authenticated transport |
| `proto/discord-v2/`, `shared/discord-v2/`, `internal/backend/discord/` | Typed Discord domain/events/helpers and host adapter |
| `internal/backend/discord/http/` | Host-only HTTP transport (package `http`, imported as `discordhttp`) |
| `internal/backend/discord/voicejobs/` | Optional bounded voice jobs; native DAVE bridge belongs to the [published SDK](https://github.com/chatanium/discordgo/tree/v0.30.0/internal/dave) |
| `shared/media/opus/` | Module-side bounded FFmpeg/Opus processing, not a core service |
| `examples/general-bot/`, `modules/music-bot/`, `modules/audio-player/` | Ordinary bot example and independent optional media modules |
| `tests/integration/` | Integration runner (`main.go`), `sandbox-policy.py` observer and `fixtures/sandbox/` |
| `docs/archive/` | Historical voice design and implementation records |

The old v1 wrappers/schema, legacy utility conversion path and experimental
helper-service implementation are removed from the v2 execution path.

Host implementation packages intentionally live under `internal/`; module authors
use public `shared/` and `proto/` packages from the unchanged
`github.com/thirdscam/chatanium-flexmodule` module. Folder names do not change
manifest identity, operator-assigned ModuleIDs, OCI labels or module grants.

## Pinned builds and independent artifacts

The language floor is Go 1.27.0; the compiler is pinned to **Go 1.27.3** in
`.go-version` and `tools/versions.mk`. Supply an actual Go executable through
`BOOTSTRAP_GO`, or install the pinned distribution at `.tools/go/bin/go`.
Version-manager shims are not accepted as compiler executables. The scripts
select the pinned toolchain, neutralize inherited `GOROOT`, and use read-only module files.

`make tools` installs Buf 1.75.0, protoc-gen-go v1.36.12 and
protoc-gen-go-grpc v1.6.2 under `.tools/bin`. From the repository root:

```sh
make tools
make build
make build-media
make buf
make buf-check
```

| Command | Artifact or scope |
|---|---|
| `make build` | `bin/runtime`, `bin/flexmodule-supervisor`, `bin/test-module`; Linux/amd64 and CGO=0 by default, no native DAVE or media tools |
| `make build-media` | `bin/chatanium-musicbot`, `bin/voice-player-module`; Go compilation only, no media tool installation |
| `make build-all` | All three local Go modules, including root packages and ordinary/media binaries; SDK checks belong to its own repository, not the native voice bundle |
| `make build-native BUILD_JOBS=2` | Independent native C++/ABI/loader/license gate; `.tools/native/prefix` and `.tools/native/bundle`, no application Go compilation or `bin/voice-runtime` publication |
| `make build-voice BUILD_JOBS=2` | `bin/voice-runtime/runtime` plus `lib/`, `licenses/` and `provenance/`; explicit voice/CGO build |
| `make buf` / `make buf-check` | Transactional owned protobuf publication / non-mutating generation comparison; see [protocol maintenance](proto/README.md) |
| `make build-integration` | `bin/flexmodule-integration` from `tests/integration` and the CLI-package `bin/flexmodule-runtime-return-holder`; CGO=0, no test execution |
| `make build-sandbox` | `bin/sandbox-module` from `tests/integration/fixtures/sandbox` |
| `make check-offline` | Buf schema/generation comparison and bounded ordinary tests in all three local Go modules; no SDK-repository, live Discord or native build |
| `make clean` | Enumerated ordinary binaries only; no Docker prune or deletion of retained native work |

The three local Go modules are `.`, `modules/music-bot`, and
`modules/audio-player`. All directly require the public
[`github.com/chatanium/discordgo v0.30.0`](https://github.com/chatanium/discordgo/tree/v0.30.0)
without a DiscordGo `replace` or local-source fallback. The immutable tag is commit
`f2d1806d4599ff674163be703c0d62a23d4bb524`, with canonical Go checksum
`h1:RDJvQXn0dO0mJxhwNT7MWybNKyS9tzvYw2cmuDYoGuI=`.
This fork includes upstream `f43dd94faaacd5b163e9e783f14b5bd8be639fc9`
plus the Gateway/context/ingress/observer and native DAVE patch set; it is not
unmodified upstream. [`tools/sdk.py`](tools/sdk.py) resolves and verifies the
consuming module's exact published source/checksum. SDK root and nested
`examples/linked_roles` / `examples/voice_receive` modules and their CI are owned
by the separate SDK repository.

Source moves preserve output names: `examples/general-bot` builds
`bin/test-module`, `modules/music-bot` builds `bin/chatanium-musicbot`, and
`modules/audio-player` builds `bin/voice-player-module`.

Native/media artifact builds require Linux/amd64, Docker Engine with BuildKit
supporting `ADD --checksum` and `COPY --chmod`, Python 3.8+, GNU
`readelf`, `ldd`, `timeout`, `flock`, `setsid`, and the target
`/lib64/ld-linux-x86-64.so.2`. Keep the checkout and workspaces below `/home`.
Large source/dependency/compiler/cache trees stay in `.tools/native` or
`.tools/media`, not host `/tmp` or Docker compiler layers. The SDK's
[`tools/native/native-lock.json`](https://github.com/chatanium/discordgo/blob/v0.30.0/tools/native/native-lock.json)
and this repository's `tools/media/tool-lock.json` pin compiler bases, package
inputs and sources. Builds do not install host packages or prune/restart Docker.

The optional native bundle uses an EL9 builder and pinned libdave commit
`6396c90aedbb9a6ee117ea9ce1ce9832af6a3375` with the approved SDK
[`libdave.patch`](https://github.com/chatanium/discordgo/blob/v0.30.0/tools/native/libdave.patch).
This repository retains the EL9 builder and bundle packaging; it resolves the
published SDK via `tools/sdk.py` and mounts that source at `/sdk`.
It is a **source-patched fork**, not an upstream libdave release binary.
Publication requires its actual ELF providers, eager relocation and target loader
checks; a distro glibc version alone is not an ABI compatibility guarantee.
Keep the complete voice bundle together. A native build does not prove encrypted
Discord interoperability.

### Module images

Set these variables to real writable OCI repository/tag names before invoking:

```sh
make image-module MODULE_IMAGE="${MODULE_IMAGE:?set an ordinary module image tag}"
make image-module MODULE_KIND=sandbox \
  MODULE_IMAGE="${SANDBOX_IMAGE:?set a sandbox fixture image tag}"
make image-media \
  MUSIC_IMAGE="${MUSIC_IMAGE:?set a music module image tag}" \
  PLAYER_IMAGE="${PLAYER_IMAGE:?set a player module image tag}" BUILD_JOBS=2
```

Ordinary images are static Go on `scratch`, without FFmpeg, yt-dlp, Deno or
native DAVE. Media images share a pinned tools layer with fixed executables:
`/opt/flexmodule/bin/ffmpeg`, `/opt/flexmodule/bin/ffprobe`,
`/opt/flexmodule/bin/deno`, and the complete
`/opt/flexmodule/yt-dlp/yt-dlp` onedir distribution. No host PATH lookup or runtime
updater is used. DCA is not the v2 playback path.

The host owns module temporary storage: it mounts scratch at
`/var/lib/flexmodule/scratch` and injects `TMPDIR` at bootstrap. Module images
must not declare `TMPDIR`, `FLEXMODULE_PLUGIN`, or any `PLUGIN_*` variable;
the host rejects even matching image defaults before container creation.
The media tools image defaults `HOME` to that scratch mount and `DENO_DIR` to
its `deno` child, never `/scratch`. After authenticated INIT, media subprocesses
explicitly use generated instance-owned scratch/cache children for HOME,
TMPDIR, Deno cache and working directories rather than relying on image defaults.
Builder-only `TMPDIR=/work/tmp` stays in the bind-mounted build workspace;
it is not a permitted module-image runtime environment declaration.

`make image-media` runs the complete build via `tools/build/images.sh media`
and `tools/build/images.py`, including the pinned tool producer. Its completed
tools-image context is `.tools/media/stage`, with payload in `stage/rootfs`;
the tools layer is packaged with `tools/media/Tools.Dockerfile` before both
module images inherit it through `tools/media/Module.Dockerfile`. For an
image-environment-only correction, an operator can reuse a previously completed,
verified staged payload and package it with the current Tools Dockerfile without
recompiling unchanged tools. Do not reuse the stale staged `Dockerfile`;
refresh changed source hashes in `opt/flexmodule/provenance/builder-source-sha256.json`
and the payload inventory `files-sha256.json` under `stage/rootfs` before
publication, retaining the actual tool-build provenance. Rebuild inheriting
module images and record their new image IDs/registry digests; existing images
do not acquire corrected environment declarations retroactively.

Each executable embeds its own canonical `manifest.json`; the builder strictly
parses it and copies the exact bytes into
`org.chatanium.flexmodule.manifest.v2`. The runtime compares the authenticated
module manifest with that image declaration. Declarations are requirements,
**not approvals**. `.tools/images/*-image.json` records local IDs separately from
registry RepoDigests. Publish through your approved registry workflow and use the
actual `registry/repository@sha256:...` RepoDigest in installation config; a local
tag or image ID is not a substitute.

## Prepare a host

Builds do not provision a host. `tools/host/prepare.py` is an explicit EL9 lab
provisioner for rootful Docker **29.5.3**, cgroup v2/systemd, runsc release
`release-20261026.0`, and XFS project quotas. It requires administrator-installed
system tools including `conntrack-tools`, `xfsprogs`, `zstd`, `iptables-nft`,
`iproute`, `util-linux`, `curl`, `shadow-utils` and `systemd`.
Missing dual-stack forwarding or firewall hooks are reported, not silently fixed.

Review the script and install it as root-owned mode 0755 at
`/opt/flexmodule-lab/prepare.py`, below root-owned, non-group/world-writable
directories with no symlink components. Do not execute a user-writable checkout
as root. Supply `/etc/flexmodule-lab/network.json` under the same trust rules.
That operator inventory must contain exactly:

- `ipv4_pool`, `ipv6_pool`: dedicated canonical, noncolliding module CIDRs.
- `resolvers`: actual public numeric resolver addresses.
- `host_public_addresses`: actual host/public/NAT addresses to exclude.
- `internal_prefixes`, `metadata_prefixes`: actual protected destination ranges.
- `inventory_revision`: an explicit finite revision string.

All inventory lists are mandatory and nonempty. Documentation addresses, guessed
public IPs and copied workstation ranges are not deployment inventory.
Set every bound below to an approved positive integer. This command performs
read-only preflight; repeat it with the exact `--apply` flag only to authorize
the documented host mutations:

```sh
sudo /usr/bin/python3 -I /opt/flexmodule-lab/prepare.py \
  --inventory /etc/flexmodule-lab/network.json \
  --max-message-bytes "${SUPERVISOR_MAX_BYTES:?set packet bound}" \
  --max-connections "${SUPERVISOR_MAX_CONNECTIONS:?set connection bound}" \
  --operation-timeout-seconds "${HOST_OPERATION_SECONDS:?set operation bound}" \
  --reconcile-interval-seconds "${HOST_RECONCILE_SECONDS:?set reconciliation interval}" \
  --api-deadline-seconds "${HOST_API_SECONDS:?set Docker API deadline}" \
  --termination-deadline-seconds "${HOST_TERMINATION_SECONDS:?set termination deadline}" \
  --deadline-seconds "${HOST_PREPARE_SECONDS:?set whole-command deadline}"
```

The source enforces each flag's finite range. Apply creates/verifies the dedicated
`flexmodule-runtime` account, a 32 GiB backing XFS image beneath
`/home/flexmodule-lab-storage`, and its `prjquota,nodev,nosuid` mount at
`/var/lib/flexmodule-lab/data`. It writes root-only
`/etc/flexmodule-lab/supervisor.json`, registers the named Docker runtime
`flexmodule-lab-runsc-20261026`, validates the merged Docker config, and uses
SIGHUP rather than restarting Docker. It does not start services, install
packages, change fstab/sysctl/firewall rules, reformat existing data or adopt
permission-mismatched directories. Reapplication requires module networks to be
absent, because collision checks also inspect workload routes.

The mount contains separate `management`, `modules`, `ipc` and `discord`
directories. The Discord ownership directory is runtime-owned mode 0700 and
outside module storage/IPC mounts; do not substitute the root-owned management
directory. Inspect generated UID/GID values rather than assuming numbers.
Install reviewed runtime/supervisor binaries into a root-protected executable
directory, for example `/opt/flexmodule-lab/bin`, before service launch.
Start the supervisor independently, as root, in its own terminal/service:

```sh
sudo /opt/flexmodule-lab/bin/flexmodule-supervisor \
  -config /etc/flexmodule-lab/supervisor.json
```

The runtime must not belong to the Docker group or receive the Docker socket.
It connects only to the supervisor socket, whose lab path is
`/var/lib/flexmodule-lab/data/management/control.sock`. The supervisor owns
cleanup even if the runtime dies. Before each Docker container/network create,
it durably records `SUBMITTED` with a deterministic name bound to the owner
triple. Any create error conservatively retains that record. A later NotFound
does not prove the request never committed: cleanup reports
`CREATE_OUTCOME_UNRESOLVED` / `UNRESOLVED_CREATE` and retains storage, quota and
the installation lease across restart. Reconciliation must first observe the
exact object ID with all ownership labels and persist `OBSERVED`; only subsequent
confirmed absence can release its cleanup obligations. It does not adopt/restart
objects or expose a force-release shortcut. Actual delayed-Docker fault timing
still requires separate exercised evidence.

## Configure and run

There is no `.env`/`private.env` autoload or `${...}` interpolation in module JSON.
`example.env` documents a host-only secret binding, not a complete runtime config.
For Discord, configure `backends.discord.major` as `2` and
`secret_sources.bot_token` as `env:FLEXMODULE_DISCORD_BOT_TOKEN`; inject the real
value into the **runtime process only** using your private service environment.
Never put it in config, image labels, argv, module environment or logs.
Provisioned user credentials use explicit purpose/resource/expiry bindings and
opaque refs; the bot token cannot be provisioned to a module.

### Operator inputs

Prepare three separate files before `generate-config`:

1. A valid runtime base JSON with `schema_version: 2`, `modules: []`, and the
   host/backend policy below. It is not the supervisor JSON.
2. Module-scoped JSON matching the chosen [ordinary](examples/general-bot/README.md),
   [music](modules/music-bot/README.md), or
   [player](modules/audio-player/README.md) contract, containing approved real IDs.
3. A strict protobuf-JSON `core-v2.Grants` approval with exact capabilities,
   resource scopes, resource profile and finite limits. Do not copy an image's
   requested `allResources` ceiling into approval to make initialization pass.

All input/output paths must be canonical absolute, non-symlink regular files,
owned by root or the runtime UID, and not group/world writable. The runtime
bounds operator files to 16 MiB. Install the final config so that the dedicated
runtime user can read it; the lab's root-only supervisor-config directory does
not automatically grant runtime access.

The authoritative JSON field names and validation rules are in
[`internal/runtime/config.go`](internal/runtime/config.go) and [`cmd/flexmodule/discord.go`](cmd/flexmodule/discord.go):

| Runtime base object | Required policy |
|---|---|
| `host` | Observed `docker_endpoint`, `docker_engine_version`, `runsc_runtime_name`, `runsc_version`, `runsc_sha256`, `management_directory`, `storage_root`, `ipc_root`; `quota_backend` is `xfs-project`. Match the independently prepared supervisor. |
| `host.network` | `firewall_backend` is `iptables`; `ipv4` and `ipv6` true; explicit `resolvers`, `host_public_addresses`, `internal_prefixes`, `infrastructure_prefixes`, `metadata_destinations`. Include actual infrastructure exclusions, not just the module pool. |
| `backends` | `{}` is backendless; only `discord` major 2 is composed by the current executable. No automatic Discord or voice registration. |
| `resource_profiles` | Optional explicit profiles with `cpu_millicores`, `memory_bytes`, `max_pids`, `persistent_bytes`, `scratch_bytes`; all finite and positive, storage in 512-byte multiples. Built-ins: general 0.5 CPU/256 MiB/64 tasks/1 GiB persistent/256 MiB scratch; media 2 CPU/1 GiB/128 tasks/5 GiB persistent/2 GiB scratch. Profiles do not grant internet or voice. |
| `defaults` | Optional positive `startup_deadline`, `drain_deadline`, `event_queue_events`, `event_queue_bytes`; defaults are 30s, 10s, 256 and 32 MiB. |
| `limits` | All fields listed below are required; no missing-to-unlimited fallback. |

`limits` duration strings: `termination_deadline`, `docker_api_deadline`,
`hook_rpc_deadline`, `readiness_poll_interval`, `credential_ref_max_ttl`,
`upload_deadline`. Positive integer bounds: `control_message_max_bytes`,
`config_max_bytes`, `control_rpcs_per_instance`, `credential_refs_per_instance`,
`persistent_inode_limit`, `scratch_inode_limit`, `log_buffer_bytes`,
`log_file_max_bytes`, `log_retained_files`, `upload_records_per_instance`,
`upload_records_per_host`, `upload_file_name_max_bytes`,
`upload_content_type_max_bytes`. Control framing must fit a 256 KiB upload chunk
and metadata; config framing must fit below that control bound. Log workspace
must exceed `3 * control_message_max_bytes + 65536 + 4096 + 256`.
Readiness polling must be shorter than startup; per-instance upload record
capacity cannot exceed the host capacity.

For `backends.discord.config`, supply `application_id`, `bot_user_id`,
`ownership_directory`, numeric `intents`, independent
`gateway_frame_max_bytes` and `gateway_decoded_max_bytes`, duration strings
`credential_ttl`, `interaction_ttl` (at most 15m), `nonce_ttl`, and positive
`nonces_per_instance`, `nonces_per_host`, `nonce_chunks_max`, `routes_per_module`,
`routes_per_host`, `max_routes_per_request`, `max_files_per_request` (at most 10),
`file_max_bytes`, `request_max_bytes`.
Its `http` object requires duration strings `request_deadline`,
`rate_limit_retention` and bounds `json_max_bytes`, `response_max_bytes`,
`header_max_bytes`, `requests_per_instance`, `requests_per_host`,
`rate_limit_buckets`. `presence_owner` and `provisioned_secrets` are optional
explicit policy, not inferred module rights.

Omit `voice` for ordinary bots. The ordinary runtime rejects voice configuration.
For the separate voice artifact, use the exact mandatory voice/transport JSON
fields in [`internal/backend/discord/voicejobs/config.go`](internal/backend/discord/voicejobs/config.go)
and the SDK transport config; those duration fields use numeric nanoseconds, not
the runtime's duration-string encoding. Voice control and packet limits must
match the runtime/transport bounds. No media tool or native dependency belongs in
the ordinary module image.

### Generate an installation

Run the generator as the dedicated runtime user, against the already running
supervisor. Variables below are operator inputs, not sample IDs or digests:

```sh
sudo -u flexmodule-runtime -- /opt/flexmodule-lab/bin/runtime generate-config \
  -base "${RUNTIME_BASE:?set absolute base JSON path}" \
  -module-config "${MODULE_CONFIG:?set absolute module JSON path}" \
  -approval "${MODULE_APPROVAL:?set absolute Grants JSON path}" \
  -image "${MODULE_REPODIGEST:?set actual immutable registry RepoDigest}" \
  -module-id "${MODULE_ID:?set stable installation identity}" \
  -entrypoint /opt/flexmodule/module \
  -uid "${MODULE_UID:?set approved nonzero module UID}" \
  -gid "${MODULE_GID:?set approved nonzero module GID}" \
  -supervisor-socket /var/lib/flexmodule-lab/data/management/control.sock \
  -supervisor-uid 0 \
  -supervisor-max-bytes "${SUPERVISOR_MAX_BYTES:?use the prepared packet bound}"
```

The exact digest must already exist in the supervisor's local image store.
The generator verifies its strict v2 image manifest, preserves separately supplied
approval, revalidates the resulting installation and writes JSON to stdout.
Capture it into a private staging file, then install it atomically with explicit
ownership/permissions; it never overwrites your config. It currently generates
one module from an empty base. For multiple installations, compose the `modules`
array using the same schema and independent approvals; do not rerun it against a
nonempty base. Optional `-config-revision` and `-approval-revision` record operator
revisions. Provisioned credential bindings require the explicit schema fields;
the generator does not invent them.

With a real token already exported in the invoking administrator's private
environment, an ordinary Discord runtime can be started as:

```sh
sudo --preserve-env=FLEXMODULE_DISCORD_BOT_TOKEN -u flexmodule-runtime -- \
  /opt/flexmodule-lab/bin/runtime run \
  -config "${RUNTIME_CONFIG:?set absolute installed runtime JSON path}" \
  -supervisor-socket /var/lib/flexmodule-lab/data/management/control.sock \
  -supervisor-uid 0 \
  -supervisor-max-bytes "${SUPERVISOR_MAX_BYTES:?use the prepared packet bound}"
```

For backendless operation omit secret preservation; for optional voice use the
complete installed voice bundle's `runtime` executable instead. `SIGINT`/`SIGTERM`
drain instances before backend/upload cleanup. `-duration` provides an optional
finite run interval, not interactive acceptance proof. `make run` exposes the
same runtime flags through `CONFIG`, `SUPERVISOR_SOCKET`, `SUPERVISOR_UID` and
`SUPERVISOR_MAX_BYTES`, but does not start/provision the supervisor.

## Generic modules, scopes and validation boundaries

[`general-bot`](examples/general-bot/README.md) demonstrates ordinary slash commands,
forms, settings, guild/DM message events, streaming attachments, and explicitly
approved member/role/channel moderation and audit operations.
[`music-bot`](modules/music-bot/README.md) and
[`audio-player`](modules/audio-player/README.md) are separate optional
media examples, not the definition of a FlexModule module.

Capabilities, actual resource scopes, Discord bot permissions, user command
permissions and Gateway intents are separate checks. Required capability refusal
prevents activation; optional refusal disables only the corresponding feature
when the module contract permits it. The ordinary basic path needs no privileged
intent, internet grant or voice capability. Module IDs remain stable across runs;
instance IDs, credentials and upload refs do not.

For `GuildMemberEdit`, an absent `roles` field preserves assignments. A present
roles list, including an explicit empty list, replaces the full set: the backend
checks existing and submitted roles and requires separately protected guild-wide
authority because Discord offers no atomic assignment-version precondition.
This is not an atomic snapshot guarantee. Use `GuildMemberRoleAdd`/`Remove` for
granular approved role changes; those operations do not inherit the full
replacement requirement.

The proposals retain their original review metadata and historical audits:
[P0 build baseline](docs/proposals/flexmodule-0-build-and-dependency-baseline.md),
[P1 protocols](docs/proposals/flexmodule-1-protocol-contracts.md),
[P2 conversions](docs/proposals/flexmodule-2-domain-conversions.md),
[P3 Discord](docs/proposals/flexmodule-3-discord-backend.md),
[P4 runtime/isolation](docs/proposals/flexmodule-4-runtime-and-isolation.md),
[P5 voice](docs/proposals/flexmodule-5-voice-jobs.md),
[P6 examples/acceptance](docs/proposals/flexmodule-6-examples-and-validation.md).
Their pre-cutover descriptions of "current v1" are historical motivation, not
instructions to use removed sources. This README describes current source
interfaces, not a claim that every proposal acceptance case has passed.

The bounded validation entry points are:

```sh
make check-offline CHECK_DEADLINE=1800s TEST_TIMEOUT=120s
make check-sandbox CONFIG="${RUNTIME_CONFIG:?}" \
  SUPERVISOR_SOCKET="${SUPERVISOR_SOCKET:?}" SUPERVISOR_UID="${SUPERVISOR_UID:?}" \
  SUPERVISOR_MAX_BYTES="${SUPERVISOR_MAX_BYTES:?}" CHECK_DEADLINE=120s OBSERVE=15s
make smoke-discord CONFIG="${RUNTIME_CONFIG:?}" \
  SUPERVISOR_SOCKET="${SUPERVISOR_SOCKET:?}" SUPERVISOR_UID="${SUPERVISOR_UID:?}" \
  SUPERVISOR_MAX_BYTES="${SUPERVISOR_MAX_BYTES:?}" CHECK_DEADLINE=180s OBSERVE=60s
make smoke-voice CONFIG="${RUNTIME_CONFIG:?}" \
  SUPERVISOR_SOCKET="${SUPERVISOR_SOCKET:?}" SUPERVISOR_UID="${SUPERVISOR_UID:?}" \
  SUPERVISOR_MAX_BYTES="${SUPERVISOR_MAX_BYTES:?}" CHECK_DEADLINE=180s OBSERVE=60s
```

Scenario configs must match the selected ordinary/sandbox/media image and actual
approvals. `smoke-voice` needs the separately built voice artifact.
`tests/integration` launches the real runtime through the real supervisor, never
an unsandboxed module fallback. Its per-case JSON distinguishes observations from
`NOT_RUN`; activation alone does not prove moderation, attachment delivery,
network isolation or encrypted audio. Missing full sandbox-policy or interactive
Discord/participant proof remains nonpassing, not an invented success.

No new build, host-isolation or Trashbin text/voice end-to-end success is asserted
by this documentation update. Live acceptance requires separately recorded
operator-approved browser actions and, for voice, actual encrypted playback and
receive observations with participants.


## Local Discord browser debugging

`.mcp.json` registers `discord-devtools` using Chrome DevTools MCP. It requires
Node.js LTS and npm, and attaches to an existing browser at
`http://127.0.0.1:9223`; it does not launch the browser or the bot runtime.

The local workstation has a separate `chatanium-discord-chromium` container:

```sh
docker start chatanium-discord-chromium
docker stop chatanium-discord-chromium
```

Its authenticated remote desktop is available at `https://localhost:3302`.
Accept the self-signed certificate only for the intended desktop address.
Desktop credentials are kept out of this repository. The browser opens
`https://discord.com/login`, where the account owner signs in manually.
The `chatanium-discord-chromium-profile` Docker volume retains the browser profile
across container stops. The existing Firefox container and its profile are separate.

The workstation's sandbox-preserving browser launcher is stored at
`~/.local/share/chatanium-discord-debug/chromium-params`. CDP is published only on
host loopback; do not expose it to the LAN or Internet, because it grants control
of the signed-in browser.

Use `list_network_requests` and `get_network_request` to inspect HTTP traffic.
MCP usage statistics and CrUX reporting are disabled. Sensitive network-header
redaction is enabled, but request/response bodies are not automatically redacted:
do not export login payloads, account tokens, or private messages.
