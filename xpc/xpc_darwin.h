#pragma once

#import "../internal/osversion/virtualization_helper.h"
#import <xpc/xpc.h>

// MARK: - dispatch_queue_t

void *dispatchQueueCreateSerial(const char *label);
void dispatchRelease(void *queue);

// MARK: - xpc.h types
//
// The following types are listed in the same order as the XPC documentation index page.
// https://developer.apple.com/documentation/xpc?language=objc

// MARK: - xpc_listener_t (macOS 14+)

void *xpcListenerCreate(const char *service_name, void *queue, uint64_t flags, uintptr_t cgo_session_handler, void **error_out);
const char *xpcListenerCopyDescription(void *listener);
bool xpcListenerActivate(void *listener, void **error_out);
void xpcListenerCancel(void *listener);
void xpcListenerRejectPeer(void *session, const char *reason);
// int xpcListenerSetPeerCodeSigningRequirement(void *listener, const char *requirement);

// MARK: - xpc_session_t (XPC_TYPE_SESSION) (macOS 13+)

// void *xpcSessionCreateXpcService(const char *service_name, void *queue, uint64_t flags, void **error_out);
void *xpcSessionCreateMachService(const char *service_name, void *queue, uint64_t flags, void **error_out);
// void xpcSessionSetTargetQueue(void *session, void *queue);
const char *xpcSessionCopyDescription(void *session);
bool xpcSessionActivate(void *session, void **error_out);
void xpcSessionSetIncomingMessageHandler(void *session, uintptr_t cgo_message_handler);
void xpcSessionCancel(void *session);
void xpcSessionSetCancelHandler(void *session, uintptr_t cgo_cancel_handler);
// void *xpcSessionSendMessage(void *session, void *message);
void xpcSessionSendMessageWithReplyAsync(void *session, void *message, uintptr_t cgo_reply_handler);
// void *xpcSessionSendMessageWithReplySync(void *session, void *message, void **error_out);

// MARK: - xpc_rich_error_t (XPC_TYPE_RICH_ERROR)
bool xpcRichErrorCanRetry(void *err);
const char *xpcRichErrorCopyDescription(void *err);

// MARK: - Identity

// # xpc_type_t
xpc_type_t xpcGetType(void *object);
const char *xpcTypeGetName(xpc_type_t type);
// # xpc_object_t
// size_t xpxHash(void *object);

// MARK: - Comparison

// # xpc_object_t
// bool xpcEqual(void *object1, void *object2);

// MARK: - Copying

// # xpc_object_t
// void *xpcCopy(void *object);
const char *xpcCopyDescription(void *object);

// MARK: - Boolean objects

// # xpc_object_t (XPC_TYPE_BOOL)
void *xpcBoolCreate(bool value);
bool xpcBoolGetValue(void *object);
void *xpcBoolTrue(); // XPC_BOOL_TRUE
void *xpcBoolFalse(); // XPC_BOOL_FALSE

// MARK: - Data objects

// # xpc_object_t (XPC_TYPE_DATA)
void *xpcDataCreate(const void *bytes, size_t length);
// size_t xpcDataGetBytes(void *object, void *buffer, size_t offset, size_t length);
const void *xpcDataGetBytesPtr(void *object);
size_t xpcDataGetLength(void *object);

// MARK: - Number objects

// # xpc_object_t (XPC_TYPE_DOUBLE)
void *xpcDoubleCreate(double value);
double xpcDoubleGetValue(void *object);

// # xpc_object_t (XPC_TYPE_INT64)
void *xpcInt64Create(int64_t value);
int64_t xpcInt64GetValue(void *object);

// # xpc_object_t (XPC_TYPE_UINT64)
void *xpcUInt64Create(uint64_t value);
uint64_t xpcUInt64GetValue(void *object);

// MARK: - Array objects

// # xpc_object_t (XPC_TYPE_ARRAY)
void *xpcArrayCreate(void *const *object, size_t count);
// void *xpcArrayCreateEmpty();
// void *xpcArrayCreateConnection
void *xpcArrayGetValue(void *object, size_t index);
void xpcArraySetValue(void *object, size_t index, void *value);
void xpcArrayAppendValue(void *object, void *value);
size_t xpcArrayGetCount(void *object);
bool xpcArrayApply(void *object, uintptr_t cgo_applier);
int xpcArrayDupFd(void *object, size_t index);
void *xpcArrayGetArray(void *object, size_t index);
bool xpcArrayGetBool(void *object, size_t index);
const void *xpcArrayGetData(void *object, size_t index, size_t *length);
int64_t xpcArrayGetDate(void *object, size_t index);
void *xpcArrayGetDictionary(void *object, size_t index);
double xpcArrayGetDouble(void *object, size_t index);
int64_t xpcArrayGetInt64(void *object, size_t index);
const char *xpcArrayGetString(void *object, size_t index);
uint64_t xpcArrayGetUInt64(void *object, size_t index);
const uint8_t *xpcArrayGetUUID(void *object, size_t index);
void xpcArraySetBool(void *object, size_t index, bool value);
// void xpcArraySetConnection
void xpcArraySetData(void *object, size_t index, const void *bytes, size_t length);
void xpcArraySetDate(void *object, size_t index, int64_t value);
void xpcArraySetDouble(void *object, size_t index, double value);
void xpcArraySetFd(void *object, size_t index, int fd);
void xpcArraySetInt64(void *object, size_t index, int64_t value);
void xpcArraySetString(void *object, size_t index, const char *string);
void xpcArraySetUInt64(void *object, size_t index, uint64_t value);
void xpcArraySetUUID(void *object, size_t index, const uuid_t uuid);
// XPC_ARRAY_APPEND

// MARK: - Dictionary objects

// # xpc_object_t (XPC_TYPE_DICTIONARY)
// void *xpcDictionaryCreate(const char *const *keys, void *const *values, size_t count);
void *xpcDictionaryCreateEmpty(void);
// void *xpcDictionaryCreateConnection
void *xpcDictionaryCreateReply(void *object);
void xpcDictionarySetValue(void *object, const char *key, void *value);
size_t xpcDictionaryGetCount(void *object);
void *xpcDictionaryGetValue(void *object, const char *key);
bool xpcDictionaryApply(void *object, uintptr_t cgo_applier);
int xpcDictionaryDupFd(void *object, const char *key);
void *xpcDictionaryGetArray(void *object, const char *key);
bool xpcDictionaryGetBool(void *object, const char *key);
const void *xpcDictionaryGetData(void *object, const char *key, size_t *length);
int64_t xpcDictionaryGetDate(void *object, const char *key);
void *xpcDictionaryGetDictionary(void *object, const char *key);
double xpcDictionaryGetDouble(void *object, const char *key);
int64_t xpcDictionaryGetInt64(void *object, const char *key);
// void *xpcDictionaryGetRemoteConnection
const char *xpcDictionaryGetString(void *object, const char *key);
uint64_t xpcDictionaryGetUInt64(void *object, const char *key);
const uint8_t *xpcDictionaryGetUUID(void *object, const char *key);
void xpcDictionarySetBool(void *object, const char *key, bool value);
// void xpcDictionarySetConnection
void xpcDictionarySetData(void *object, const char *key, const void *bytes, size_t length);
void xpcDictionarySetDate(void *object, const char *key, int64_t value);
void xpcDictionarySetDouble(void *object, const char *key, double value);
void xpcDictionarySetFd(void *object, const char *key, int fd);
void xpcDictionarySetInt64(void *object, const char *key, int64_t value);
void xpcDictionarySetString(void *object, const char *key, const char *value);
void xpcDictionarySetUInt64(void *object, const char *key, uint64_t value);
void xpcDictionarySetUUID(void *object, const char *key, const uint8_t *uuid);
// void *xpcDictionaryCopyMachSend
// void xpcDictionarySetMachSend

// MARK: - String objects

// # xpc_object_t (XPC_TYPE_STRING)
void *xpcStringCreate(const char *string);
// void *xpcStringCreateWithFormat(const char *format, ...);
// void *xpcStringCreateWithFormatAndArguments(const char *format, va_list args);
size_t xpcStringGetLength(void *object);
const char *xpcStringGetStringPtr(void *object);

// MARK: - File Descriptor objects

// # xpc_object_t (XPC_TYPE_FD)
void *xpcFdCreate(int fd);
int xpcFdDup(void *object);

// MARK: - Date objects

// # xpc_object_t (XPC_TYPE_DATE)
void *xpcDateCreate(int64_t interval);
void *xpcDateCreateFromCurrent();
int64_t xpcDateGetValue(void *object);

// MARK: - UUID objects

// # xpc_object_t (XPC_TYPE_UUID)
void *xpcUUIDCreate(const uuid_t uuid);
const uint8_t *xpcUUIDGetBytes(void *object);

// MARK: - Shared Memory objects

// # xpc_object_t (XPC_TYPE_SHMEM)
// void *xpcShmemCreate(void *region, size_t length);
// size_t xpcShmemMap(void *object, void **region);

// MARK: - Null objects
// # xpc_object_t (XPC_TYPE_NULL)
void *xpcNullCreate();

// MARK: - Object life cycle
void *xpcRetain(void *object);
void xpcRelease(void *object);

// MARK: - xpc_peer_requirement_t (macOS 26+)
void xpcListenerSetPeerRequirement(void *listener, void *peer_requirement);
// void *xpcPeerRequirementCreateEntitlementExists(const char *entitlement, void **error_out);
// void *xpcPeerRequirementCreateEntitlementMatchesValue(const char *entitlement, void *value, void **error_out);
void *xpcPeerRequirementCreateLwcr(void *lwcr, void **error_out);
// void *xpcPeerRequirementCreatePlatformIdentity(const char * signing_identifier, void **error_out);
// void *xpcPeerRequirementCreateTeamIdentity(const char * team_identifier, void **error_out);
bool xpcPeerRequirementMatchReceivedMessage(void *peer_requirement, void *message, void **error_out);
void xpcSessionSetPeerRequirement(void *session, void *peer_requirement);
