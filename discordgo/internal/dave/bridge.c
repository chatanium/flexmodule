//go:build voice && cgo

#include "bridge.h"
#include <stdatomic.h>

struct flex_dave_failure {
    atomic_bool failed;
};

static void flex_dave_mls_failure(const char* source, const char* reason, void* user_data)
{
    (void)source;
    (void)reason;
    atomic_store_explicit(&((flex_dave_failure*)user_data)->failed, true, memory_order_release);
}

static void flex_dave_discard_log(DAVELoggingSeverity severity, const char* file,
                                int line, const char* message)
{
    (void)severity;
    (void)file;
    (void)line;
    (void)message;
}

flex_dave_failure* flex_dave_failure_create(void)
{
    flex_dave_failure* state = malloc(sizeof(*state));
    if (state != NULL) {
        atomic_init(&state->failed, false);
    }
    return state;
}

void flex_dave_failure_destroy(flex_dave_failure* state)
{
    free(state);
}

void flex_dave_failure_reset(flex_dave_failure* state)
{
    atomic_store_explicit(&state->failed, false, memory_order_release);
}

bool flex_dave_failure_failed(flex_dave_failure* state)
{
    return atomic_load_explicit(&state->failed, memory_order_acquire);
}

DAVESessionHandle flex_dave_session_create(flex_dave_failure* state)
{
    /* Empty authSessionId selects transient identity. No persistent key ID or
     * platform key-store context is supplied. */
    return daveSessionCreate(NULL, NULL, flex_dave_mls_failure, state);
}

void flex_dave_suppress_logs(void)
{
    /* Upstream verbose logs include MLS bytes and key-domain addresses. Never
     * route those messages through the host logger, including error text. */
    daveSetLogSinkCallback(flex_dave_discard_log);
}
