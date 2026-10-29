//go:build voice && cgo

#ifndef CHATANIUM_DAVE_BRIDGE_H
#define CHATANIUM_DAVE_BRIDGE_H

#include <dave/dave.h>
#include <dave/flex_dave_ext.h>
#include <stdlib.h>

/* Only C-owned memory is retained by native callbacks. The callback records no
 * source, reason, identity or key-bearing text. All session calls are serialized
 * by Context; the MLS callbacks used here execute synchronously in those calls. */
typedef struct flex_dave_failure flex_dave_failure;
flex_dave_failure* flex_dave_failure_create(void);
void flex_dave_failure_destroy(flex_dave_failure* state);
void flex_dave_failure_reset(flex_dave_failure* state);
bool flex_dave_failure_failed(flex_dave_failure* state);
DAVESessionHandle flex_dave_session_create(flex_dave_failure* state);
void flex_dave_suppress_logs(void);

#endif
