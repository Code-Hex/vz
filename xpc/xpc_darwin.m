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

// # xpc_object_t (XPC_TYPE_BOOL)

void *xpcBoolCreate(bool value)
{
    return xpc_bool_create(value);
}

bool xpcBoolGetValue(void *object)
{
    return xpc_bool_get_value((xpc_object_t)object);
}

void *xpcBoolTrue()
{
    return XPC_BOOL_TRUE;
}

void *xpcBoolFalse()
{
    return XPC_BOOL_FALSE;
}

// MARK: - Data objects

// # xpc_object_t (XPC_TYPE_DATA)

void *xpcDataCreate(const void *bytes, size_t length)
{
    return xpc_data_create(bytes, length);
}

const void *xpcDataGetBytesPtr(void *object)
{
    return xpc_data_get_bytes_ptr((xpc_object_t)object);
}

size_t xpcDataGetLength(void *object)
{
    return xpc_data_get_length((xpc_object_t)object);
}

// MARK: - Number objects

// # xpc_object_t (XPC_TYPE_DOUBLE)
void *xpcDoubleCreate(double value)
{
    return xpc_double_create(value);
}

double xpcDoubleGetValue(void *object)
{
    return xpc_double_get_value((xpc_object_t)object);
}

// MARK: - Int64 objects
// # xpc_object_t (XPC_TYPE_INT64)
void *xpcInt64Create(int64_t value)
{
    return xpc_int64_create(value);
}

int64_t xpcInt64GetValue(void *object)
{
    return xpc_int64_get_value((xpc_object_t)object);
}

// MARK: - UInt64 objects
// # xpc_object_t (XPC_TYPE_UINT64)
void *xpcUInt64Create(uint64_t value)
{
    return xpc_uint64_create(value);
}

uint64_t xpcUInt64GetValue(void *object)
{
    return xpc_uint64_get_value((xpc_object_t)object);
}

// MARK: - Array objects

// # xpc_object_t (XPC_TYPE_ARRAY)

void *xpcArrayCreate(void *const *object, size_t count)
{
    return xpc_array_create((xpc_object_t const *)object, count);
}

void *xpcArrayGetValue(void *object, size_t index)
{
    return xpc_array_get_value((xpc_object_t)object, index);
}

void xpcArraySetValue(void *object, size_t index, void *value)
{
    xpc_array_set_value((xpc_object_t)object, index, (xpc_object_t)value);
}

void xpcArrayAppendValue(void *object, void *value)
{
    xpc_array_append_value((xpc_object_t)object, (xpc_object_t)value);
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

int xpcArrayDupFd(void *object, size_t index)
{
    return xpc_array_dup_fd((xpc_object_t)object, index);
}

void *xpcArrayGetArray(void *object, size_t index)
{
    return xpc_array_get_array((xpc_object_t)object, index);
}

bool xpcArrayGetBool(void *object, size_t index)
{
    return xpc_array_get_bool((xpc_object_t)object, index);
}

const void *xpcArrayGetData(void *object, size_t index, size_t *length)
{
    return xpc_array_get_data((xpc_object_t)object, index, length);
}

int64_t xpcArrayGetDate(void *object, size_t index)
{
    return xpc_array_get_date((xpc_object_t)object, index);
}

void *xpcArrayGetDictionary(void *object, size_t index)
{
    return xpc_array_get_dictionary((xpc_object_t)object, index);
}

double xpcArrayGetDouble(void *object, size_t index)
{
    return xpc_array_get_double((xpc_object_t)object, index);
}

int64_t xpcArrayGetInt64(void *object, size_t index)
{
    return xpc_array_get_int64((xpc_object_t)object, index);
}

const char *xpcArrayGetString(void *object, size_t index)
{
    return xpc_array_get_string((xpc_object_t)object, index);
}

uint64_t xpcArrayGetUInt64(void *object, size_t index)
{
    return xpc_array_get_uint64((xpc_object_t)object, index);
}

const uint8_t *xpcArrayGetUUID(void *object, size_t index)
{
    return xpc_array_get_uuid((xpc_object_t)object, index);
}

void xpcArraySetBool(void *object, size_t index, bool value)
{
    xpc_array_set_bool((xpc_object_t)object, index, value);
}

void xpcArraySetData(void *object, size_t index, const void *bytes, size_t length)
{
    xpc_array_set_data((xpc_object_t)object, index, bytes, length);
}

void xpcArraySetDate(void *object, size_t index, int64_t value)
{
    xpc_array_set_date((xpc_object_t)object, index, value);
}

void xpcArraySetDouble(void *object, size_t index, double value)
{
    xpc_array_set_double((xpc_object_t)object, index, value);
}

void xpcArraySetFd(void *object, size_t index, int fd)
{
    xpc_array_set_fd((xpc_object_t)object, index, fd);
}

void xpcArraySetInt64(void *object, size_t index, int64_t value)
{
    xpc_array_set_int64((xpc_object_t)object, index, value);
}

void xpcArraySetString(void *object, size_t index, const char *value)
{
    xpc_array_set_string((xpc_object_t)object, index, value);
}

void xpcArraySetUInt64(void *object, size_t index, uint64_t value)
{
    xpc_array_set_uint64((xpc_object_t)object, index, value);
}

void xpcArraySetUUID(void *object, size_t index, const uint8_t *uuid)
{
    xpc_array_set_uuid((xpc_object_t)object, index, uuid);
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

size_t xpcDictionaryGetCount(void *object)
{
    return xpc_dictionary_get_count((xpc_object_t)object);
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

int xpcDictionaryDupFd(void *object, const char *key)
{
    return xpc_dictionary_dup_fd((xpc_object_t)object, key);
}

void *xpcDictionaryGetArray(void *object, const char *key)
{
    return xpc_dictionary_get_array((xpc_object_t)object, key);
}

bool xpcDictionaryGetBool(void *object, const char *key)
{
    return xpc_dictionary_get_bool((xpc_object_t)object, key);
}

const void *xpcDictionaryGetData(void *object, const char *key, size_t *length)
{
    return xpc_dictionary_get_data((xpc_object_t)object, key, length);
}

int64_t xpcDictionaryGetDate(void *object, const char *key)
{
    return xpc_dictionary_get_date((xpc_object_t)object, key);
}

void *xpcDictionaryGetDictionary(void *object, const char *key)
{
    return xpc_dictionary_get_dictionary((xpc_object_t)object, key);
}

double xpcDictionaryGetDouble(void *object, const char *key)
{
    return xpc_dictionary_get_double((xpc_object_t)object, key);
}

int64_t xpcDictionaryGetInt64(void *object, const char *key)
{
    return xpc_dictionary_get_int64((xpc_object_t)object, key);
}

const char *xpcDictionaryGetString(void *object, const char *key)
{
    return xpc_dictionary_get_string((xpc_object_t)object, key);
}

uint64_t xpcDictionaryGetUInt64(void *object, const char *key)
{
    return xpc_dictionary_get_uint64((xpc_object_t)object, key);
}

const uint8_t *xpcDictionaryGetUUID(void *object, const char *key)
{
    return xpc_dictionary_get_uuid((xpc_object_t)object, key);
}

void xpcDictionarySetBool(void *object, const char *key, bool value)
{
    xpc_dictionary_set_bool((xpc_object_t)object, key, value);
}

void xpcDictionarySetData(void *object, const char *key, const void *bytes, size_t length)
{
    xpc_dictionary_set_data((xpc_object_t)object, key, bytes, length);
}

void xpcDictionarySetDate(void *object, const char *key, int64_t value)
{
    xpc_dictionary_set_date((xpc_object_t)object, key, value);
}

void xpcDictionarySetDouble(void *object, const char *key, double value)
{
    xpc_dictionary_set_double((xpc_object_t)object, key, value);
}

void xpcDictionarySetFd(void *object, const char *key, int fd)
{
    xpc_dictionary_set_fd((xpc_object_t)object, key, fd);
}

void xpcDictionarySetInt64(void *object, const char *key, int64_t value)
{
    xpc_dictionary_set_int64((xpc_object_t)object, key, value);
}

void xpcDictionarySetString(void *object, const char *key, const char *value)
{
    xpc_dictionary_set_string((xpc_object_t)object, key, value);
}

void xpcDictionarySetUInt64(void *object, const char *key, uint64_t value)
{
    xpc_dictionary_set_uint64((xpc_object_t)object, key, value);
}

void xpcDictionarySetUUID(void *object, const char *key, const uint8_t *uuid)
{
    xpc_dictionary_set_uuid((xpc_object_t)object, key, uuid);
}

// MARK: - String objects

void *xpcStringCreate(const char *string)
{
    return xpc_string_create(string);
}

size_t xpcStringGetLength(void *object)
{
    return xpc_string_get_length((xpc_object_t)object);
}

const char *xpcStringGetStringPtr(void *object)
{
    return xpc_string_get_string_ptr((xpc_object_t)object);
}

// MARK: - File descriptor objects

void *xpcFdCreate(int fd)
{
    return xpc_fd_create(fd);
}

int xpcFdDup(void *object)
{
    return xpc_fd_dup((xpc_object_t)object);
}

// MARK: - Date objects

void *xpcDateCreate(int64_t interval)
{
    return xpc_date_create(interval);
}

void *xpcDateCreateFromCurrent()
{
    return xpc_date_create_from_current();
}

int64_t xpcDateGetValue(void *object)
{
    return xpc_date_get_value((xpc_object_t)object);
}

// MARK: - UUID objects

void *xpcUUIDCreate(const uuid_t uuid)
{
    return xpc_uuid_create(uuid);
}

const uint8_t *xpcUUIDGetBytes(void *object)
{
    return xpc_uuid_get_bytes((xpc_object_t)object);
}

// MARK: - Shared memory objects
// MARK: - Null objects

void *xpcNullCreate()
{
    return xpc_null_create();
}

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