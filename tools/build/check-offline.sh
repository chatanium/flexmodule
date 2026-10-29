#!/usr/bin/env bash
set -euo pipefail
source "$(dirname -- "${BASH_SOURCE[0]}")/../common.sh"
use_go
check_tools
: "${CHECK_DEADLINE:=1800s}" "${TEST_TIMEOUT:=120s}"
export CHECK_DEADLINE TEST_TIMEOUT
[[ "$CHECK_DEADLINE" =~ ^[1-9][0-9]*[smh]$ && "$TEST_TIMEOUT" =~ ^[1-9][0-9]*[smh]$ ]] || fail 'Check deadlines require positive integer seconds/minutes/hours (for example 120s), never zero'
# These vendored upstream tests have live opt-in environment gates. Offline
# means they cannot accidentally connect using credentials in the host shell.
unset DG_OAUTH2_TOKEN DGB_TOKEN DGU_TOKEN DG_GUILD DG_CHANNEL DG_VOICE_CHANNEL DG_ADMIN DISCORD_TOKEN
# Every retained module is checked, including both independent SDK examples.
# Whole-suite and per-package deadlines are independent and failures propagate.
run timeout --signal=TERM --kill-after=10s "$CHECK_DEADLINE" bash -euo pipefail -c '
    .tools/bin/buf build proto
    bash tools/generate.sh check
    for module in . discordgo discordgo/examples/linked_roles discordgo/examples/voice_receive chatanium-musicbot voice-player-module; do
        printf "Checking offline module %s\n" "$module"
        "$1" -C "$module" test -mod=readonly -tags= -count=1 -timeout "$TEST_TIMEOUT" ./...
    done
' bash "$GO_BINARY"
