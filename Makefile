include tools/versions.mk

.DEFAULT_GOAL := build
GOOS ?= linux
GOARCH ?= amd64
CGO_ENABLED ?= 0
export GOOS GOARCH CGO_ENABLED GO_TOOLCHAIN BUF_VERSION PROTOC_GEN_GO_VERSION PROTOC_GEN_GO_GRPC_VERSION

# Six maintained modules; helper-service and helper-service/plugin-go-grpc are
# ignored experiments, not production build inputs. Each descriptor is
# module:package:output; '-' means compile packages into Go's build cache only.
BUILD_TARGETS := .:.:bin/runtime .:./test-module:bin/test-module
MEDIA_TARGETS := chatanium-musicbot:.:bin/chatanium-musicbot voice-player-module:.:bin/voice-player-module
MODULE_CHECK_TARGETS := .:./...:-
# ./... includes all SDK packages, tools and non-nested SDK examples. Nested
# module examples need their own builds and have distinct published binaries.
SDK_TARGETS := discordgo:./...:- \
	discordgo/examples/linked_roles:.:bin/discordgo-linked-roles \
	discordgo/examples/voice_receive:.:bin/discordgo-voice-receive
BUILD_OUTPUTS := bin/runtime bin/test-module bin/chatanium-musicbot \
	bin/voice-player-module bin/discordgo-linked-roles bin/discordgo-voice-receive

# Exact ownership, never a wildcard. At schema cutover retain obsolete owned
# paths here until one successful publication removes them; EXPECTED lists only
# outputs produced by the new schemas. Unregistered .pb.go files are untouched.
GENERATED_FILES := common.pb.go \
	core-v1/helper.pb.go core-v1/helper_grpc.pb.go \
	core-v1/hook.pb.go core-v1/hook_grpc.pb.go \
	discord-v1/discordgo.pb.go \
	discord-v1/helper.pb.go discord-v1/helper_grpc.pb.go \
	discord-v1/hook.pb.go discord-v1/hook_grpc.pb.go \
	discord-v1/voice.pb.go discord-v1/voice_grpc.pb.go
GENERATED_EXPECTED_FILES := $(GENERATED_FILES)
export GENERATED_FILES GENERATED_EXPECTED_FILES BUILD_OUTPUTS

.PHONY: tools build build-media build-all buf buf-check clean run

tools:
	@bash tools/install-tools.sh

build:
	@bash tools/build.sh build $(BUILD_TARGETS)

# Go compilation only: no ffmpeg, yt-dlp, Discord connection or native DAVE.
# If a target requires CGO, use CGO_ENABLED=1 with an explicit matching CC and
# installed system libraries; cross-compilation needs a target C toolchain.
build-media:
	@bash tools/build.sh build $(MEDIA_TARGETS)

build-all:
	@bash tools/build.sh build $(MODULE_CHECK_TARGETS) $(BUILD_TARGETS) $(SDK_TARGETS) $(MEDIA_TARGETS)

buf:
	@bash tools/generate.sh publish

buf-check:
	@bash tools/generate.sh check

clean:
	@bash tools/build.sh clean

run: build
	@./bin/runtime

# P5 adds build-voice only with its real native builder and complete bundle.
