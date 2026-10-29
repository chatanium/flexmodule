# Protocol definitions and generated bindings

This directory owns FlexModule v2 wire contracts and their generated Go gRPC
bindings. The runtime and modules use the wrappers in
[`../shared`](../shared/README.md); generated messages alone do not implement
authorization, sandbox bootstrap or lifecycle.

## Current sources

| Source | Contract |
|---|---|
| `core-v2/core.proto` | Backend-neutral manifest, requirements/actual grants, resource scopes, exact JSON configuration, Hook lifecycle/events, credential/file refs and streaming FileTransfer |
| `discord-v2/domain.proto` | Typed Discord snapshots, presence-aware partials, nullable/list/map wrappers and closed component variants |
| `discord-v2/events.proto` | Concrete safe event DTOs, packed into the core event envelope with a registered type URL |
| `discord-v2/helper.proto` | Canonical typed Discord helper operations; convenience aliases are SDK recipes, not extra RPCs |
| `discord-v2/ownership.proto` | Exact interaction-route ownership and provisioned credential-ref operations |
| `discord-v2/voice.proto` | Optional bounded voice playback/receive job service, separate from ordinary helpers |

There are no current `hooks.proto`/`helpers.proto` entry points or v1 compatibility
bindings. Application handshake/manifest major 2 must agree; runtime and modules
are rebuilt together. The authenticated primary Hook transport has one fixed,
lifetime-scoped reverse-helper connection, shared by the core/file/backend
services. Adding a backend does not add a broker connection.

The schemas do not serialize `discordgo.Session`, live `State`,
`VoiceConnection`, file readers or raw host credentials. File bytes use explicit
streaming refs. Interaction/webhook credentials use typed opaque references.
Absent, zero, false, empty, clear and explicit JSON null remain distinct; enum
and permission-mask values are not renumbered to declaration ordinals.

`StageRequest.init_context` supplies the authenticated `persistent_directory`
and `scratch_directory` paths. Consumers must use those values rather than
assuming literal mount locations; host implementation constants are not
replacement configuration.

## Regenerate Go bindings

From the repository root, with the pinned Go compiler installed:

```sh
make tools
make buf
make buf-check
```

[`buf.gen.yaml`](buf.gen.yaml) uses the pinned local
`.tools/bin/protoc-gen-go` and `protoc-gen-go-grpc`, with
`paths=source_relative` and `require_unimplemented_servers=false`.
`make buf` runs Buf through `tools/generate.sh` into staging, validates the exact
owned output set from `Makefile`, then publishes transactionally. Failure before
publication preserves existing outputs; partial publication is rolled back.
`make buf-check` compares staged outputs without changing checked-in files.
Neither command recursively deletes unrelated `.pb.go` files.

Buf consumes the checked-in `.proto` sources. It does **not** refresh SDK source
inventories, emit schemas from source policy, regenerate domain conversions or
rebuild helper clients/backend planners.

## Maintain source-derived contracts

The local SDK is pinned to upstream `f43dd94` plus explicit local Gateway/native
voice patches. Reviewed inventories and policy are under
`tools/protocol/inventory/`; historical v0.29.0/v1 audits remain provenance, not
the current API ceiling. After an intentional SDK/source-policy change, update
the reviewed policy and source provenance rather than weakening inventory
checks. The dependent generation order is:

```sh
.tools/go/bin/go run -mod=readonly ./tools/protocol/extract-source.go \
  -sdk discordgo -out tools/protocol/inventory/source-extracted.json
python3 tools/protocol/refresh-inventory.py
python3 tools/protocol/emit-domain.py
python3 tools/protocol/generate_events.py
python3 tools/protocol/generate-api.py
make buf
python3 tools/protocol/generate-conversions.py
python3 tools/protocol/generate-client.py
python3 tools/protocol/generate-backend-api.py
```

The first command assumes the pinned distribution is installed at
`.tools/go/bin/go`; use the actual pinned compiler path if you chose another
installation. Run these commands from the repository root. Review all generated
source/mapping changes together; this sequence is not a replacement for the
parent's formatting, build, test and live acceptance work.

| Generator | Owned inputs/outputs |
|---|---|
| `extract-source.go`, `refresh-inventory.py` | Current SDK declarations plus reviewed source policy to refreshed API/event/type inventories; historical audits are not overwritten |
| `emit-domain.py` | Domain schema and `domain-mapping.json` from the reviewed type inventory |
| `generate_events.py` | Event schema, typed event registry and `event-mapping.json` |
| `generate-api.py` | Helper schema, operation registry and `api-mapping.json` |
| `generate-conversions.py` | Safe views, conversions, ingress/projection support and conversion coverage from mappings and generated message shapes |
| `generate-client.py` | Typed SDK calls, explicit aliases and client coverage using the canonical API mapping and safe converter types |
| `generate-backend-api.py` | Host helper handlers, fixed REST plans/codecs/projections and source-derived planning fixtures/coverage |

`core.proto`, `ownership.proto` and `voice.proto` remain explicit contracts;
source inventory generation does not infer grants or redefine those services.
Mapping/coverage JSON describes reviewed source decisions and generated scope,
not proof that runtime authorization, Docker isolation, Discord mutations or
encrypted voice passed. See the root [P0-P6 links and validation
boundaries](../README.md#generic-modules-scopes-and-validation-boundaries).
