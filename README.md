# FlexModule
**Next Generation Module System for Chatanium**<br/>
We're completely stripping out Chatanium's existing Go Plugin-based module system and creating a new one here.

## Goals

![FlexModule Diagram](https://github.com/user-attachments/assets/1cf77e2c-ea04-47be-b2c5-1d4bb7bd9fc6)

Once our PoC implementation for multi-provider (Backend) is finalized, this codebase will be adapted to Chatanium v2.

## Design

* **Module system integration based on gRPC (w. protobuf)**<br/>
Ultimately, Chatanium's goal is to deploy/develop/operate better chatbots. Therefore, the existing module system has some limitations (https://github.com/thirdscam/chatanium/issues/10), and fixing them is our top priority. To solve these problems, we will implement a module system based on gRPC, which will provide cross-platform compilation and better security.

## Build and generation

The language floor is Go 1.24.0. The compiler is pinned to **Go 1.27.3** in
`.go-version` and `tools/versions.mk`; build commands select it with `GOTOOLCHAIN`.
Buf 1.50.0, protoc-gen-go v1.36.5, and protoc-gen-go-grpc v1.3.0 are installed
under the ignored `.tools/bin` directory. If a version-manager shim cannot start
the pinned compiler, set `BOOTSTRAP_GO` to an actual Go executable, or install
the pinned distribution at `.tools/go/bin/go`.

```sh
make tools
make build
make build-media
make buf
make buf-check
```

| Command | Owned scope |
|---|---|
| `make build` | `bin/runtime` and `bin/test-module`; no native voice dependency |
| `make build-media` | Go binaries for `chatanium-musicbot` and `voice-player-module`; does not install or invoke ffmpeg or yt-dlp |
| `make build-all CGO_ENABLED=1` | All six maintained modules, including all root/SDK packages and both nested SDK examples |
| `make buf` | Generate into staging, validate the exact output set, and publish only owned generated files |
| `make buf-check` | Compare staged generation with checked-in outputs without changing them |
| `make clean` | Remove only the explicitly listed build outputs; preserve unrelated files in `bin/` |
| `make run` | Build successfully before launching the runtime with the operator's configuration |

The maintained modules are the root, `discordgo`, `chatanium-musicbot`,
`voice-player-module`, `discordgo/examples/linked_roles`, and
`discordgo/examples/voice_receive`. Every DiscordGo consumer resolves to the
same local `discordgo` tree. The two ignored `helper-service` modules are
experiments, not production build inputs.

The legacy SDK voice-receive example requires a C compiler, pkg-config, Opus,
and opusfile development files. On Rocky Linux these are provided by
`opus-devel`, `opusfile-devel`, and their dependencies; enable CGO explicitly
for that full inventory build. Linux/amd64 and `CGO_ENABLED=0` are the ordinary
defaults. Cross-compiling a CGO target additionally requires a matching `CC`
and target libraries.

Builds use read-only module files and publish each binary only after its
compilation succeeds. Failed generation leaves existing generated files
unchanged; a publication failure restores already replaced owned files.
Generation and build publication are serialized independently. The optional
native DAVE host bundle is a separate P5 deliverable, not an implicit part of
ordinary or media-module compilation.

### Implementation record

- **2026-10-29 — P0 baseline:** pinned tools and local SDK resolution, restored
  interaction converter compilation, and replaced destructive build/generation
  commands. Ordinary and media binaries plus the complete six-module inventory
  compiled. Deterministic regeneration, non-mutating drift detection, failed
  build preservation, failed generation preservation, and rollback after a
  partially published change were exercised. These checks do not certify v2
  conversion semantics, sandbox isolation, or live Discord/voice behavior.
- The [staged proposals](docs/proposals/flexmodule-0-build-and-dependency-baseline.md)
  define the coordinated v2 cutover. The P0 runtime still uses v1; it is not a
  completed or deployable v2 implementation.

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
