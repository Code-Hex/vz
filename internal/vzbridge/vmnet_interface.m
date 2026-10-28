#import "native.h"
#import <Foundation/Foundation.h>
#import <stdio.h>
#import <vmnet/vmnet.h>
#if __MAC_OS_X_VERSION_MAX_ALLOWED >= 260000
struct VmnetSession {
    interface_ref interface;
    dispatch_queue_t events;
    dispatch_block_t disableCallback;
    uint64_t readMaxPackets, writeMaxPackets;
};
static void interfaceError(vmnet_return_t status, NSString *operation, void **output)
{
    if (status != VMNET_SUCCESS && output)
        *output = [[NSError errorWithDomain:@"com.apple.vmnet" code:status userInfo:@ { NSLocalizedDescriptionKey : [operation stringByAppendingString:@" failed"] }] retain];
}
#endif

void *vz_startVmnetInterface(void *network, const char *name, uint32_t options, uint64_t callback, uint64_t context, void *maxPacketSize, void *mac, void **errorOut)
{
#if __MAC_OS_X_VERSION_MAX_ALLOWED >= 260000
    if (name && !name[0])
        name = NULL;
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
        if (options & 16)
            xpc_dictionary_set_bool(desc, vmnet_enable_virtio_header_key, true);
        else
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
                uint64_t maximum = xpc_dictionary_get_uint64(parameters, vmnet_max_packet_size_key);
                // vmnet reports the frame limit without the virtio header.
                // Leave invalid limits intact for the Go startup validation.
                if ((options & 16) && maximum > 0 && maximum <= INT32_MAX)
                    maximum += 12;
                *(uint64_t *)maxPacketSize = maximum;
                session->readMaxPackets = xpc_dictionary_get_uint64(parameters, vmnet_read_max_packets_key);
                session->writeMaxPackets = xpc_dictionary_get_uint64(parameters, vmnet_write_max_packets_key);
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
#endif
    return NULL;
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
int32_t vz_writeVmnetPacket(void *pointer, const void *buffer, uint64_t size, void **errorOut)
{
#if __MAC_OS_X_VERSION_MAX_ALLOWED >= 260000
    if (@available(macOS 26, *)) {
        struct VmnetSession *session = pointer;
        struct iovec vector = { .iov_base = (void *)buffer, .iov_len = size };
        struct vmpktdesc packet = { .vm_pkt_size = size, .vm_pkt_iov = &vector, .vm_pkt_iovcnt = 1, .vm_flags = 0 };
        int count = 1;
        vmnet_return_t result = vmnet_write(session->interface, &packet, &count);
        interfaceError(result, @"vmnet_write", errorOut);
        return result == VMNET_SUCCESS ? count : 0;
    }
#endif
    return 0;
}

// vmnet limits each call to 256 KB. Use decimal KB conservatively.
#if __MAC_OS_X_VERSION_MAX_ALLOWED >= 260000
static int32_t transferPackets(void *pointer, void *buffers, void *sizes, int32_t requested, bool reading, void **errorOut)
{
    struct VmnetSession *session = pointer;
    uint64_t limit = reading ? session->readMaxPackets : session->writeMaxPackets;
    if (!limit)
        limit = 1;
    if (requested < 1 || requested > 32) {
        interfaceError(VMNET_INVALID_ARGUMENT, @"packet batch count", errorOut);
        return 0;
    }
    struct iovec vectors[32];
    struct vmpktdesc packets[32];
    uint64_t *lengths = sizes;
    uint64_t remaining = 256000;
    int submitted = 0;
    for (; submitted < requested && submitted < limit; submitted++) {
        uint64_t size = lengths[submitted];
        if (!size || size > remaining)
            break;
        remaining -= size;
        vectors[submitted] = (struct iovec) { .iov_base = ((void **)buffers)[submitted], .iov_len = size };
        packets[submitted] = (struct vmpktdesc) { .vm_pkt_size = size, .vm_pkt_iov = &vectors[submitted], .vm_pkt_iovcnt = 1, .vm_flags = 0 };
    }
    if (!submitted) {
        interfaceError(VMNET_INVALID_ARGUMENT, @"packet batch size", errorOut);
        return 0;
    }
    int count = submitted;
    vmnet_return_t result = reading ? vmnet_read(session->interface, packets, &count)
                                    : vmnet_write(session->interface, packets, &count);
    interfaceError(result, reading ? @"vmnet_read" : @"vmnet_write", errorOut);
    // An error may leave count unchanged. Only successful calls report progress.
    if (result != VMNET_SUCCESS)
        return 0;
    if (count < 0 || count > submitted) {
        interfaceError(VMNET_FAILURE, @"packet batch result count", errorOut);
        return 0;
    }
    if (reading)
        for (int i = 0; i < count; i++)
            lengths[i] = packets[i].vm_pkt_size;
    return count;
}
#endif

int32_t vz_readVmnetPackets(void *pointer, void *buffers, void *sizes, int32_t count, void **errorOut)
{
#if __MAC_OS_X_VERSION_MAX_ALLOWED >= 260000
    if (@available(macOS 26, *))
        return transferPackets(pointer, buffers, sizes, count, true, errorOut);
#endif
    return 0;
}

int32_t vz_writeVmnetPackets(void *pointer, void *buffers, void *sizes, int32_t count, void **errorOut)
{
#if __MAC_OS_X_VERSION_MAX_ALLOWED >= 260000
    if (@available(macOS 26, *))
        return transferPackets(pointer, buffers, sizes, count, false, errorOut);
#endif
    return 0;
}
