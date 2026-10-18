#import <Foundation/Foundation.h>
#import <vmnet/vmnet.h>
#import <assert.h>

#if __MAC_OS_X_VERSION_MAX_ALLOWED >= 260000
static vmnet_return_t unregisterStatus, stopStatus, completionStatus;
static vmnet_return_t startStatus = VMNET_SUCCESS, registrationStatus = VMNET_SUCCESS;
static int stops, callbacks, frees;
static void (^savedCallback)(interface_event_t, xpc_object_t);

static void countFree(void *pointer)
{
    frees++;
    free(pointer);
}

static interface_ref startInterface(void *network, xpc_object_t description, dispatch_queue_t queue, void (^completion)(vmnet_return_t, xpc_object_t))
{
    dispatch_async(queue, ^{ completion(startStatus, NULL); });
    return (interface_ref)(uintptr_t)1;
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
    uint8_t mac[6] = {0};
    void *error = NULL;
    struct VmnetSession *session = vz_startVmnetInterface(NULL, 0, (uint64_t)wake, 1, &size, mac, &error);
    assert(session && !error);
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
    uint8_t mac[6] = {0};
    void *error = NULL;
    struct VmnetSession *session = vz_startVmnetInterface(NULL, 0, (uint64_t)wake, 1, &size, mac, &error);
    assert(session && error);
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
            checkStop(VMNET_SUCCESS, VMNET_SUCCESS, VMNET_SUCCESS, VMNET_SUCCESS);
            checkStop(VMNET_FAILURE, VMNET_SUCCESS, VMNET_SUCCESS, VMNET_FAILURE);
            checkStop(VMNET_SUCCESS, VMNET_INVALID_ARGUMENT, VMNET_SUCCESS, VMNET_INVALID_ARGUMENT);
            checkStop(VMNET_SUCCESS, VMNET_SUCCESS, VMNET_FAILURE, VMNET_FAILURE);
            checkStop(VMNET_FAILURE, VMNET_INVALID_ARGUMENT, VMNET_SUCCESS, VMNET_INVALID_ARGUMENT);
            checkStartupFailure(VMNET_FAILURE, VMNET_SUCCESS);
            checkStartupFailure(VMNET_SUCCESS, VMNET_FAILURE);
        }
        return 0;
    }
#endif
    return 77;
}
