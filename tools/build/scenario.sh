#!/usr/bin/env bash
set -euo pipefail
source "$(dirname -- "${BASH_SOURCE[0]}")/../common.sh"
scenario=${1:?Expected sandbox, discord or voice}
case "$scenario" in sandbox|discord|voice) ;; *) fail 'Unknown validation scenario' ;; esac
: "${CONFIG:?Set CONFIG to an actual approved absolute operator JSON path}" \
  "${SUPERVISOR_SOCKET:?Set SUPERVISOR_SOCKET to prepared supervisor UDS}" \
  "${SUPERVISOR_UID:?Set SUPERVISOR_UID to trusted supervisor UID}" \
  "${SUPERVISOR_MAX_BYTES:?Set SUPERVISOR_MAX_BYTES to finite control bound}" \
  "${CHECK_DEADLINE:?Set CHECK_DEADLINE to finite total scenario duration}" \
  "${OBSERVE:?Set OBSERVE to finite observation duration}"
[[ "$CHECK_DEADLINE" =~ ^[1-9][0-9]*[smh]$ && "$OBSERVE" =~ ^[1-9][0-9]*[smh]$ ]] || fail 'Scenario durations require positive integer seconds/minutes/hours (for example 60s), never zero'
[[ "$CONFIG" == /* && -f "$CONFIG" ]] || fail 'CONFIG must be an existing absolute path'
[[ "$SUPERVISOR_SOCKET" == /* && -S "$SUPERVISOR_SOCKET" ]] || fail 'Supervisor UDS unavailable; no fallback launcher'
[[ "$SUPERVISOR_UID" =~ ^[0-9]+$ && "$SUPERVISOR_MAX_BYTES" =~ ^[1-9][0-9]*$ ]] || fail 'Invalid supervisor UID/control bound'
use_go
runtime=${RUNTIME:-bin/runtime}
if [[ "$scenario" == voice ]]; then
    runtime=${RUNTIME:-bin/voice-runtime/runtime}
    [[ -f bin/voice-runtime/provenance/native-lock.json ]] || fail 'Build real voice-enabled native artifact with make build-voice first'
fi
[[ -x "$runtime" ]] || fail 'Build the requested runtime before smoke; no automatic installs or fake success'
# Never source .env, echo config or pass a secret into module container env.
# The real harness records unavailable live observations as NOT_RUN.
run timeout --signal=TERM --kill-after=15s "$CHECK_DEADLINE" \
    "$GO_BINARY" run -mod=readonly ./integration-tests "$scenario" \
    -config "$CONFIG" -runtime "$runtime" -deadline "$CHECK_DEADLINE" -observe "$OBSERVE" \
    -supervisor-socket "$SUPERVISOR_SOCKET" -supervisor-uid "$SUPERVISOR_UID" \
    -supervisor-max-bytes "$SUPERVISOR_MAX_BYTES"
