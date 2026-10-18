#import "native.h"
#import <Foundation/Foundation.h>
#import <stdio.h>
#import <vmnet/vmnet.h>
#if __MAC_OS_X_VERSION_MAX_ALLOWED >= 260000
struct VmnetSession {
    interface_ref interface;
    dispatch_queue_t events;
    dispatch_block_t disableCallback;
};
static void interfaceError(vmnet_return_t status, NSString *operation, void **output)
{
    if (status != VMNET_SUCCESS && output)
        *output = [[NSError errorWithDomain:@"com.apple.vmnet" code:status userInfo:@ { NSLocalizedDescriptionKey : [operation stringByAppendingString:@" failed"] }] retain];
}
static void *startVmnetInterface(void *network, const char *name, uint32_t options, uint64_t callback, uint64_t context, void *maxPacketSize, void *mac, void **errorOut)
{
    if (@available(macOS 26, *)) {
        struct VmnetSession *session = calloc(1, sizeof(*session));
        if (!session) {
            interfaceError(VMNET_MEM_FAILURE, @"allocate interface", errorOut);
            return NULL;
        }
        dispatch_queue_t completion = dispatch_queue_create("com.codehex.vz.vmnet.completion", DISPATCH_QUEUE_SERIAL);
        dispatch_semaphore_t done = dispatch_semaphore_create(0);
        xpc_object_t desc = xpc_dictionary_create(NULL, NULL, 0);
        xpc_dictionary_set_bool(desc, vmnet_allocate_mac_address_key, !(options & 1));
        xpc_dictionary_set_bool(desc, vmnet_enable_checksum_offload_key, options & 2);
        xpc_dictionary_set_bool(desc, vmnet_enable_tso_key, options & 4);
        xpc_dictionary_set_bool(desc, vmnet_enable_isolation_key, options & 8);
        if (name) {
            xpc_dictionary_set_uint64(desc, vmnet_operation_mode_key, VMNET_BRIDGED_MODE);
            xpc_dictionary_set_string(desc, vmnet_shared_interface_name_key, name);
        }
        NSString *operation = name ? @"vmnet_start_interface" : @"vmnet_interface_start_with_network";
        __block vmnet_return_t result = VMNET_FAILURE;
        vmnet_start_interface_completion_handler_t handler = ^(vmnet_return_t status, xpc_object_t parameters) {
            result = status;
            if (status == VMNET_SUCCESS && parameters) {
                *(uint64_t *)maxPacketSize = xpc_dictionary_get_uint64(parameters, vmnet_max_packet_size_key);
                const char *address = xpc_dictionary_get_string(parameters, vmnet_mac_address_key);
                unsigned int bytes[6];
                if (address && sscanf(address, "%x:%x:%x:%x:%x:%x", &bytes[0], &bytes[1], &bytes[2], &bytes[3], &bytes[4], &bytes[5]) == 6)
                    for (int i = 0; i < 6; i++)
                        ((uint8_t *)mac)[i] = bytes[i];
            }
            dispatch_semaphore_signal(done);
        };
        session->interface = name ? vmnet_start_interface(desc, completion, handler)
                                  : vmnet_interface_start_with_network(network, desc, completion, handler);
        xpc_release(desc);
        if (session->interface) {
            dispatch_semaphore_wait(done, DISPATCH_TIME_FOREVER);
            dispatch_sync(completion, ^ { });
        }
        dispatch_release(done);
        dispatch_release(completion);
        if (!session->interface) {
            interfaceError(result == VMNET_SUCCESS ? VMNET_FAILURE : result, operation, errorOut);
            free(session);
            return NULL;
        }
        session->events = dispatch_queue_create("com.codehex.vz.vmnet.events", DISPATCH_QUEUE_SERIAL);
        __block bool active = true;
        session->disableCallback = [^{ active = false; } copy];
        if (result != VMNET_SUCCESS) {
            interfaceError(result, operation, errorOut);
            return session;
        }
        result = vmnet_interface_set_event_callback(session->interface, VMNET_INTERFACE_PACKETS_AVAILABLE, session->events, ^(interface_event_t mask, xpc_object_t event) {
            if (active && (mask & VMNET_INTERFACE_PACKETS_AVAILABLE))
                ((void (*)(uint64_t))(uintptr_t)callback)(context);
        });
        if (result != VMNET_SUCCESS) {
            interfaceError(result, @"vmnet_interface_set_event_callback", errorOut);
            return session;
        }
        return session;
    }
    return NULL;
}
#endif

void *vz_startVmnetInterface(void *network, uint32_t options, uint64_t callback, uint64_t context, void *maxPacketSize, void *mac, void **errorOut)
{
#if __MAC_OS_X_VERSION_MAX_ALLOWED >= 260000
    return startVmnetInterface(network, NULL, options, callback, context, maxPacketSize, mac, errorOut);
#else
    return NULL;
#endif
}

void *vz_startVmnetBridgedInterface(const char *name, uint32_t options, uint64_t callback, uint64_t context, void *maxPacketSize, void *mac, void **errorOut)
{
#if __MAC_OS_X_VERSION_MAX_ALLOWED >= 260000
    return startVmnetInterface(NULL, name, options, callback, context, maxPacketSize, mac, errorOut);
#else
    return NULL;
#endif
}

void *vz_copyVmnetBridgedInterfaces(void **errorOut)
{
#if __MAC_OS_X_VERSION_MAX_ALLOWED >= 260000
    if (@available(macOS 26, *)) {
        xpc_object_t interfaces = vmnet_copy_shared_interface_list();
        if (!interfaces) {
            interfaceError(VMNET_FAILURE, @"vmnet_copy_shared_interface_list", errorOut);
            return NULL;
        }
        NSMutableArray *names = [[NSMutableArray alloc] init];
        for (size_t i = 0; i < xpc_array_get_count(interfaces); i++) {
            NSString *name = [[NSString alloc] initWithUTF8String:xpc_array_get_string(interfaces, i)];
            [names addObject:name];
            [name release];
        }
        xpc_release(interfaces);
        return names;
    }
#endif
    return NULL;
}

void vz_stopVmnetInterface(void *pointer, void **errorOut)
{
#if __MAC_OS_X_VERSION_MAX_ALLOWED >= 260000
    if (@available(macOS 26, *)) {
        struct VmnetSession *session = pointer;
        // Queued callbacks share this gate without retaining the session or Go handle.
        dispatch_sync(session->events, session->disableCallback);
        vmnet_return_t callbackResult = vmnet_interface_set_event_callback(session->interface, VMNET_INTERFACE_PACKETS_AVAILABLE, NULL, NULL);
        dispatch_queue_t completion = dispatch_queue_create("com.codehex.vz.vmnet.stop", DISPATCH_QUEUE_SERIAL);
        dispatch_semaphore_t done = dispatch_semaphore_create(0);
        __block vmnet_return_t status = VMNET_FAILURE;
        vmnet_return_t result = vmnet_stop_interface(session->interface, completion, ^(vmnet_return_t value) { status = value; dispatch_semaphore_signal(done); });
        if (result == VMNET_SUCCESS) {
            dispatch_semaphore_wait(done, DISPATCH_TIME_FOREVER);
            dispatch_sync(completion, ^ { });
            result = status;
        }
        dispatch_release(done);
        dispatch_release(completion);
        [session->disableCallback release];
        dispatch_release(session->events);
        if (result != VMNET_SUCCESS)
            interfaceError(result, @"vmnet_stop_interface", errorOut);
        else
            interfaceError(callbackResult, @"disable vmnet callback", errorOut);
        free(session);
    }
#endif
}
int32_t vz_readVmnetPacket(void *pointer, void *buffer, uint64_t size, void **errorOut)
{
#if __MAC_OS_X_VERSION_MAX_ALLOWED >= 260000
    if (@available(macOS 26, *)) {
        struct VmnetSession *session = pointer;
        struct iovec vec = { .iov_base = buffer, .iov_len = size };
        struct vmpktdesc packet = { .vm_pkt_size = size, .vm_pkt_iov = &vec, .vm_pkt_iovcnt = 1, .vm_flags = 0 };
        int count = 1;
        vmnet_return_t result = vmnet_read(session->interface, &packet, &count);
        interfaceError(result, @"vmnet_read", errorOut);
        return result == VMNET_SUCCESS && count ? (int32_t)packet.vm_pkt_size : 0;
    }
#endif
    return 0;
}
void vz_writeVmnetPacket(void *pointer, const void *buffer, uint64_t size, void **errorOut)
{
#if __MAC_OS_X_VERSION_MAX_ALLOWED >= 260000
    if (@available(macOS 26, *)) {
        struct VmnetSession *session = pointer;
        struct iovec vec = { .iov_base = (void *)buffer, .iov_len = size };
        struct vmpktdesc packet = { .vm_pkt_size = size, .vm_pkt_iov = &vec, .vm_pkt_iovcnt = 1, .vm_flags = 0 };
        int count = 1;
        interfaceError(vmnet_write(session->interface, &packet, &count), @"vmnet_write", errorOut);
    }
#endif
}
