#import <Foundation/Foundation.h>
#import <assert.h>
#import <vmnet/vmnet.h>

#if __MAC_OS_X_VERSION_MAX_ALLOWED >= 260000
static vmnet_return_t unregisterStatus, stopStatus, completionStatus;
static vmnet_return_t startStatus = VMNET_SUCCESS, registrationStatus = VMNET_SUCCESS;
static int stops, callbacks, frees;
static bool bridged, listFails, emptyList, startReturnsNull;
static uint32_t expectedOptions;
static uint64_t reportedMaximum = 1514, expectedMaximum = 1514;
static void (^savedCallback)(interface_event_t, xpc_object_t);

static void countFree(void *pointer)
{
    frees++;
    free(pointer);
}

static interface_ref startInterface(void *network, xpc_object_t description, dispatch_queue_t queue, void (^completion)(vmnet_return_t, xpc_object_t))
{
    assert(!bridged);
    dispatch_async(queue, ^{ completion(startStatus, NULL); });
    return (interface_ref)(uintptr_t)1;
}

static interface_ref startBridgedInterface(xpc_object_t description, dispatch_queue_t queue, void (^completion)(vmnet_return_t, xpc_object_t))
{
    assert(bridged);
    assert(xpc_dictionary_get_uint64(description, vmnet_operation_mode_key) == VMNET_BRIDGED_MODE);
    assert(strcmp(xpc_dictionary_get_string(description, vmnet_shared_interface_name_key), "en0") == 0);
    assert(xpc_dictionary_get_bool(description, vmnet_allocate_mac_address_key) == !(expectedOptions & 1));
    if (expectedOptions & 16) {
        assert(xpc_dictionary_get_bool(description, vmnet_enable_virtio_header_key));
        assert(!xpc_dictionary_get_value(description, vmnet_enable_checksum_offload_key));
    } else {
        assert(!xpc_dictionary_get_value(description, vmnet_enable_virtio_header_key));
        assert(xpc_dictionary_get_bool(description, vmnet_enable_checksum_offload_key) == !!(expectedOptions & 2));
    }
    assert(xpc_dictionary_get_bool(description, vmnet_enable_tso_key) == !!(expectedOptions & 4));
    assert(xpc_dictionary_get_bool(description, vmnet_enable_isolation_key) == !!(expectedOptions & 8));
    assert(xpc_dictionary_get_count(description) == 6);
    if (startReturnsNull)
        return NULL;
    dispatch_async(queue, ^{
        xpc_object_t parameters = xpc_dictionary_create(NULL, NULL, 0);
        xpc_dictionary_set_uint64(parameters, vmnet_max_packet_size_key, reportedMaximum);
        xpc_dictionary_set_string(parameters, vmnet_mac_address_key, "02:00:00:00:00:01");
        completion(startStatus, parameters);
        xpc_release(parameters);
    });
    return (interface_ref)(uintptr_t)1;
}

static xpc_object_t sharedInterfaceList(void)
{
    if (listFails)
        return NULL;
    xpc_object_t names = xpc_array_create(NULL, 0);
    if (!emptyList) {
        xpc_array_set_string(names, XPC_ARRAY_APPEND, "en0");
        xpc_array_set_string(names, XPC_ARRAY_APPEND, "en1");
    }
    return names;
}

static vmnet_return_t setCallback(interface_ref interface, interface_event_t mask, dispatch_queue_t queue, void (^callback)(interface_event_t, xpc_object_t))
{
    if (callback) {
        savedCallback = [callback copy];
        return registrationStatus;
    }
    return unregisterStatus;
}

static vmnet_return_t stopInterface(interface_ref interface, dispatch_queue_t queue, void (^completion)(vmnet_return_t))
{
    stops++;
    if (stopStatus == VMNET_SUCCESS)
        dispatch_async(queue, ^{ completion(completionStatus); });
    return stopStatus;
}

#define vmnet_start_interface startBridgedInterface
#define vmnet_copy_shared_interface_list sharedInterfaceList
#define vmnet_interface_start_with_network startInterface
#define vmnet_interface_set_event_callback setCallback
#define vmnet_stop_interface stopInterface
#define free countFree
#include "vmnet_interface.m"
#undef free

static void wake(uint64_t context)
{
    callbacks++;
}

static void checkStop(vmnet_return_t unregisterResult, vmnet_return_t stopResult, vmnet_return_t completionResult, vmnet_return_t expectedError)
{
    unregisterStatus = unregisterResult;
    stopStatus = stopResult;
    completionStatus = completionResult;
    stops = callbacks = frees = 0;
    uint64_t size = 0;
    uint8_t mac[6] = { 0 };
    void *error = NULL;
    struct VmnetSession *session = bridged
        ? vz_startVmnetInterface(NULL, "en0", expectedOptions, (uint64_t)wake, 1, &size, mac, &error)
        : vz_startVmnetInterface(NULL, "", 0, (uint64_t)wake, 1, &size, mac, &error);
    assert(session && !error);
    if (bridged) {
        assert(size == expectedMaximum);
        const uint8_t expectedMAC[] = { 2, 0, 0, 0, 0, 1 };
        assert(memcmp(mac, expectedMAC, sizeof(mac)) == 0);
    }
    dispatch_queue_t events = session->events;
    dispatch_retain(events);
    dispatch_async(events, ^{ savedCallback(VMNET_INTERFACE_PACKETS_AVAILABLE, NULL); });
    vz_stopVmnetInterface(session, &error);
    assert(stops == 1);
    assert(frees == 1);
    assert(callbacks == 1);
    dispatch_sync(events, ^{ savedCallback(VMNET_INTERFACE_PACKETS_AVAILABLE, NULL); });
    assert(callbacks == 1);
    if (expectedError == VMNET_SUCCESS)
        assert(error == NULL);
    else
        assert([(NSError *)error code] == expectedError);
    [(NSError *)error release];
    [savedCallback release];
    dispatch_release(events);
}

static void checkStartupFailure(vmnet_return_t startResult, vmnet_return_t registrationResult)
{
    startStatus = startResult;
    registrationStatus = registrationResult;
    unregisterStatus = stopStatus = completionStatus = VMNET_SUCCESS;
    stops = frees = callbacks = 0;
    savedCallback = nil;
    uint64_t size = 0;
    uint8_t mac[6] = { 0 };
    void *error = NULL;
    struct VmnetSession *session = bridged
        ? vz_startVmnetInterface(NULL, "en0", expectedOptions, (uint64_t)wake, 1, &size, mac, &error)
        : vz_startVmnetInterface(NULL, "", 0, (uint64_t)wake, 1, &size, mac, &error);
    assert(session && error);
    NSString *operation = startResult == VMNET_SUCCESS ? @"vmnet_interface_set_event_callback"
                                                       : (bridged ? @"vmnet_start_interface" : @"vmnet_interface_start_with_network");
    assert([[(NSError *)error localizedDescription] isEqualToString:[operation stringByAppendingString:@" failed"]]);
    [(NSError *)error release];
    error = NULL;
    dispatch_queue_t events = session->events;
    dispatch_retain(events);
    vz_stopVmnetInterface(session, &error);
    assert(!error && stops == 1 && frees == 1);
    if (savedCallback) {
        dispatch_sync(events, ^{ savedCallback(VMNET_INTERFACE_PACKETS_AVAILABLE, NULL); });
        assert(callbacks == 0);
        [savedCallback release];
    }
    dispatch_release(events);
}

#endif

int main(void)
{
#if __MAC_OS_X_VERSION_MAX_ALLOWED >= 260000
    if (@available(macOS 26, *)) {
        @autoreleasepool {
            for (int mode = 0; mode < 2; mode++) {
                bridged = mode;
                startStatus = registrationStatus = VMNET_SUCCESS;
                checkStop(VMNET_SUCCESS, VMNET_SUCCESS, VMNET_SUCCESS, VMNET_SUCCESS);
                checkStop(VMNET_FAILURE, VMNET_SUCCESS, VMNET_SUCCESS, VMNET_FAILURE);
                checkStop(VMNET_SUCCESS, VMNET_INVALID_ARGUMENT, VMNET_SUCCESS, VMNET_INVALID_ARGUMENT);
                checkStop(VMNET_SUCCESS, VMNET_SUCCESS, VMNET_FAILURE, VMNET_FAILURE);
                checkStop(VMNET_FAILURE, VMNET_INVALID_ARGUMENT, VMNET_SUCCESS, VMNET_INVALID_ARGUMENT);
                checkStartupFailure(VMNET_FAILURE, VMNET_SUCCESS);
                checkStartupFailure(VMNET_SUCCESS, VMNET_FAILURE);
            }
            startStatus = registrationStatus = VMNET_SUCCESS;
            for (expectedOptions = 0; expectedOptions < 16; expectedOptions++)
                checkStop(VMNET_SUCCESS, VMNET_SUCCESS, VMNET_SUCCESS, VMNET_SUCCESS);
            expectedOptions = 16;
            const uint64_t limits[][2] = {
                { 1514, 1526 },
                { 0, 0 },
                { INT32_MAX, (uint64_t)INT32_MAX + 12 },
                { UINT64_MAX, UINT64_MAX },
            };
            for (size_t i = 0; i < sizeof(limits) / sizeof(limits[0]); i++) {
                reportedMaximum = limits[i][0];
                expectedMaximum = limits[i][1];
                checkStop(VMNET_SUCCESS, VMNET_SUCCESS, VMNET_SUCCESS, VMNET_SUCCESS);
            }
            reportedMaximum = expectedMaximum = 1514;
            expectedOptions = 0;
            startReturnsNull = true;
            frees = 0;
            uint64_t size = 0;
            uint8_t mac[6] = { 0 };
            void *error = NULL;
            assert(vz_startVmnetInterface(NULL, "en0", 0, (uint64_t)wake, 1, &size, mac, &error) == NULL);
            assert(error && [(NSError *)error code] == VMNET_FAILURE && frees == 1);
            [(NSError *)error release];
            error = NULL;
            NSArray *names = vz_copyVmnetBridgedInterfaces(&error);
            assert((!error && [names isEqualToArray:@[ @"en0", @"en1" ]]));
            [names release];
            emptyList = true;
            names = vz_copyVmnetBridgedInterfaces(&error);
            assert(names && !error && [names count] == 0);
            [names release];
            listFails = true;
            assert(vz_copyVmnetBridgedInterfaces(&error) == NULL);
            assert(error && [(NSError *)error code] == VMNET_FAILURE);
            [(NSError *)error release];
        }
        return 0;
    }
#endif
    return 77;
}
