#include "xpc_darwin.h"

// MARK: - Helper functions defined in Go

// # xpc_object_t
extern uintptr_t wrapRawObject(void *obj);
// # xpc_listener_t
extern void callSessionHandler(uintptr_t cgoSessionHandler, uintptr_t cgoSession);

// # xpc_session_t
extern void callReplyHandler(uintptr_t cgoReplyHandler, uintptr_t cgoReply, uintptr_t cgoError);
extern void callCancelHandler(uintptr_t cgoCancelHandler, uintptr_t cgoError);
extern void *callMessageHandler(uintptr_t cgoMessageHandler, uintptr_t cgoMessage);

// # xpc_object_t (XPC_TYPE_ARRAY)
extern bool callArrayApplier(uintptr_t cgoApplier, size_t index, uintptr_t cgoValue);
// # xpc_object_t (XPC_TYPE_DICTIONARY)
extern bool callDictionaryApplier(uintptr_t cgoApplier, const char *_Nonnull key, uintptr_t cgoValue);

// MARK: -dispatch_queue_t

void *dispatchQueueCreateSerial(const char *label)
{
    return dispatch_queue_create(label, DISPATCH_QUEUE_SERIAL);
}

void dispatchRelease(void *queue)
{
    dispatch_release((dispatch_queue_t)queue);
}

// MARK: - xpc.h types
//
// The following types are listed in the same order as the XPC documentation index page.
// https://developer.apple.com/documentation/xpc?language=objc

// MARK: - xpc_listener_t (macOS 14+)

void *xpcListenerCreate(const char *service_name, void *queue, uint64_t flags, uintptr_t cgo_session_handler, void **error_out)
{
#ifdef INCLUDE_TARGET_OSX_14
    if (@available(macOS 14, *)) {
        return xpc_listener_create(
            service_name,
            queue,
            flags,
            ^(xpc_session_t _Nonnull session) {
                callSessionHandler(cgo_session_handler, wrapRawObject(session));
            },
            (xpc_rich_error_t *)error_out);
    }
#endif
    RAISE_UNSUPPORTED_MACOS_EXCEPTION();
}

const char *xpcListenerCopyDescription(void *listener)
{
#ifdef INCLUDE_TARGET_OSX_14
    if (@available(macOS 14, *)) {
        return xpc_listener_copy_description((xpc_listener_t)listener);
    }
#endif
    RAISE_UNSUPPORTED_MACOS_EXCEPTION();
}

bool xpcListenerActivate(void *listener, void **error_out)
{
#ifdef INCLUDE_TARGET_OSX_14
    if (@available(macOS 14, *)) {
        return xpc_listener_activate((xpc_listener_t)listener, (xpc_rich_error_t *)error_out);
    }
#endif
    RAISE_UNSUPPORTED_MACOS_EXCEPTION();
}

void xpcListenerCancel(void *listener)
{
#ifdef INCLUDE_TARGET_OSX_14
    if (@available(macOS 14, *)) {
        xpc_listener_cancel((xpc_listener_t)listener);
        return;
    }
#endif
    RAISE_UNSUPPORTED_MACOS_EXCEPTION();
}

void xpcListenerRejectPeer(void *session, const char *reason)
{
#ifdef INCLUDE_TARGET_OSX_14
    if (@available(macOS 14, *)) {
        xpc_listener_reject_peer((xpc_session_t)session, reason);
        return;
    }
#endif
    RAISE_UNSUPPORTED_MACOS_EXCEPTION();
}

// MARK: - xpc_session_t (XPC_TYPE_SESSION) (macOS 13+)

void *xpcSessionCreateMachService(const char *service_name, void *queue, uint64_t flags, void **error_out)
{
#ifdef INCLUDE_TARGET_OSX_13
    if (@available(macOS 13, *)) {
        return xpc_session_create_mach_service(
            service_name,
            (dispatch_queue_t)queue,
            flags,
            (xpc_rich_error_t *)error_out);
    }
#endif
    RAISE_UNSUPPORTED_MACOS_EXCEPTION();
}

const char *xpcSessionCopyDescription(void *session)
{
#ifdef INCLUDE_TARGET_OSX_13
    if (@available(macOS 13, *)) {
        return xpc_session_copy_description((xpc_session_t)session);
    }
#endif
    RAISE_UNSUPPORTED_MACOS_EXCEPTION();
}

bool xpcSessionActivate(void *session, void **error_out)
{
#ifdef INCLUDE_TARGET_OSX_13
    if (@available(macOS 13, *)) {
        return xpc_session_activate((xpc_session_t)session, (xpc_rich_error_t *)error_out);
    }
#endif
    RAISE_UNSUPPORTED_MACOS_EXCEPTION();
}

void xpcSessionSetIncomingMessageHandler(void *session, uintptr_t cgo_message_handler)
{
#ifdef INCLUDE_TARGET_OSX_13
    if (@available(macOS 13, *)) {
        xpc_session_set_incoming_message_handler(
            (xpc_session_t)session,
            ^(xpc_object_t _Nonnull message) {
                // Ensure the message is a dictionary.
                if (xpc_get_type(message) != XPC_TYPE_DICTIONARY) {
                    xpc_session_cancel((xpc_session_t)session);
                    return;
                }
                xpc_object_t reply = (xpc_object_t)callMessageHandler(cgo_message_handler, wrapRawObject(message));
                xpc_rich_error_t err;
                do {
                    err = xpc_session_send_message(session, reply);
                } while (err != nil && xpc_rich_error_can_retry(err));
                xpc_release(reply);
            });
        return;
    }
#endif
    RAISE_UNSUPPORTED_MACOS_EXCEPTION();
}

void xpcSessionCancel(void *session)
{
#ifdef INCLUDE_TARGET_OSX_13
    if (@available(macOS 13, *)) {
        xpc_session_cancel((xpc_session_t)session);
        return;
    }
#endif
    RAISE_UNSUPPORTED_MACOS_EXCEPTION();
}

void xpcSessionSetCancelHandler(void *session, uintptr_t cgo_cancel_handler)
{
#ifdef INCLUDE_TARGET_OSX_13
    if (@available(macOS 13, *)) {
        xpc_session_set_cancel_handler(
            (xpc_session_t)session,
            ^(xpc_rich_error_t _Nonnull err) {
                callCancelHandler(cgo_cancel_handler, wrapRawObject(err));
            });
        return;
    }
#endif
    RAISE_UNSUPPORTED_MACOS_EXCEPTION();
}

void xpcSessionSendMessageWithReplyAsync(void *session, void *message, uintptr_t cgo_reply_handler)
{
#ifdef INCLUDE_TARGET_OSX_13
    if (@available(macOS 13, *)) {
        xpc_session_send_message_with_reply_async(
            (xpc_session_t)session,
            (xpc_object_t)message,
            ^(xpc_object_t _Nonnull reply, xpc_rich_error_t _Nullable error) {
                callReplyHandler(cgo_reply_handler, wrapRawObject(reply), wrapRawObject(error));
            });
        return;
    }
#endif
    RAISE_UNSUPPORTED_MACOS_EXCEPTION();
}

// MARK: - xpc_rich_error_t (XPC_TYPE_RICH_ERROR)

bool xpcRichErrorCanRetry(void *err)
{
    return xpc_rich_error_can_retry((xpc_rich_error_t)err);
}

const char *xpcRichErrorCopyDescription(void *err)
{
    return xpc_rich_error_copy_description((xpc_rich_error_t)err);
}

// MARK: - Identity

// # xpc_type_t

xpc_type_t xpcGetType(void *object)
{
    return xpc_get_type((xpc_object_t)object);
}

const char *xpcTypeGetName(xpc_type_t type)
{
    return xpc_type_get_name(type);
}

// MARK: - Comparison
// MARK: - Copying

// # xpc_object_t
const char *xpcCopyDescription(void *object)
{
    return xpc_copy_description((xpc_object_t)object);
}

// MARK: - Boolean objects

// MARK: - Data objects

// # xpc_object_t (XPC_TYPE_DATA)

void *xpcDataCreate(const void *bytes, size_t length)
{
    return xpc_data_create(bytes, length);
}

// MARK: - Number objects

// MARK: - Array objects

// # xpc_object_t (XPC_TYPE_ARRAY)

void *xpcArrayCreate(void *const *object, size_t count)
{
    return xpc_array_create((xpc_object_t const *)object, count);
}

size_t xpcArrayGetCount(void *object)
{
    return xpc_array_get_count((xpc_object_t)object);
}

bool xpcArrayApply(void *object, uintptr_t cgo_applier)
{
    return xpc_array_apply((xpc_object_t)object, ^bool(size_t index, xpc_object_t _Nonnull value) {
        return callArrayApplier(cgo_applier, index, wrapRawObject(value));
    });
}

// MARK: - Dictionary objects

// xpc_object_t (XPC_TYPE_DICTIONARY)

void *xpcDictionaryCreateEmpty(void)
{
    return xpc_dictionary_create_empty();
}

void *xpcDictionaryCreateReply(void *object)
{
    return xpc_dictionary_create_reply((xpc_object_t)object);
}

void xpcDictionarySetValue(void *object, const char *key, void *value)
{
    xpc_dictionary_set_value((xpc_object_t)object, key, (xpc_object_t)value);
}

void *xpcDictionaryGetValue(void *object, const char *key)
{
    return xpc_dictionary_get_value((xpc_object_t)object, key);
}

bool xpcDictionaryApply(void *object, uintptr_t cgo_applier)
{
    return xpc_dictionary_apply((xpc_object_t)object, ^bool(const char *_Nonnull key, xpc_object_t _Nonnull value) {
        return callDictionaryApplier(cgo_applier, key, wrapRawObject(value));
    });
}

void *xpcDictionaryGetArray(void *object, const char *key)
{
    return xpc_dictionary_get_array((xpc_object_t)object, key);
}

const void *xpcDictionaryGetData(void *object, const char *key, size_t *length)
{
    return xpc_dictionary_get_data((xpc_object_t)object, key, length);
}

const char *xpcDictionaryGetString(void *object, const char *key)
{
    return xpc_dictionary_get_string((xpc_object_t)object, key);
}

void xpcDictionarySetString(void *object, const char *key, const char *value)
{
    xpc_dictionary_set_string((xpc_object_t)object, key, value);
}

// MARK: - String objects

void *xpcStringCreate(const char *string)
{
    return xpc_string_create(string);
}

// MARK: - File descriptor objects
// MARK: - Date objects
// MARK: - UUID objects
// MARK: - Shared memory objects
// MARK: - Null objects

// MARK: - Object life cycle

// xpc_object_t

void *xpcRetain(void *object)
{
    return xpc_retain((xpc_object_t)object);
}

void xpcRelease(void *object)
{
    xpc_release((xpc_object_t)object);
}

// MARK: - xpc_peer_requirement_t (macOS 26+)

void xpcListenerSetPeerRequirement(void *listener, void *peer_requirement)
{
#ifdef INCLUDE_TARGET_OSX_26
    if (@available(macOS 26, *)) {
        xpc_listener_set_peer_requirement((xpc_listener_t)listener, (xpc_peer_requirement_t)peer_requirement);
        return;
    }
#endif
    RAISE_UNSUPPORTED_MACOS_EXCEPTION();
}

void *xpcPeerRequirementCreateLwcr(void *lwcr, void **error_out)
{
#ifdef INCLUDE_TARGET_OSX_26
    if (@available(macOS 26, *)) {
        return xpc_peer_requirement_create_lwcr((xpc_object_t)lwcr, (xpc_rich_error_t *)error_out);
    }
#endif
    RAISE_UNSUPPORTED_MACOS_EXCEPTION();
}

bool xpcPeerRequirementMatchReceivedMessage(void *peer_requirement, void *message, void **error_out)
{
#ifdef INCLUDE_TARGET_OSX_26
    if (@available(macOS 26, *)) {
        return xpc_peer_requirement_match_received_message(
            (xpc_peer_requirement_t)peer_requirement,
            (xpc_object_t)message,
            (xpc_rich_error_t *)error_out);
    }
#endif
    RAISE_UNSUPPORTED_MACOS_EXCEPTION();
}

void xpcSessionSetPeerRequirement(void *session, void *peer_requirement)
{
#ifdef INCLUDE_TARGET_OSX_26
    if (@available(macOS 26, *)) {
        xpc_session_set_peer_requirement((xpc_session_t)session, (xpc_peer_requirement_t)peer_requirement);
        return;
    }
#endif
    RAISE_UNSUPPORTED_MACOS_EXCEPTION();
}