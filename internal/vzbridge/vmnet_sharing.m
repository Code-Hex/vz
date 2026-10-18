#import "native.h"
#import <Foundation/Foundation.h>
#import <xpc/xpc.h>

@interface VZNetworkServiceState : NSObject {
@public
    xpc_object_t serialization;
    NSMutableSet *peers;
    bool closed;
}
@end

@implementation VZNetworkServiceState
- (void)dealloc
{
    xpc_release(serialization);
    [peers release];
    [super dealloc];
}
@end

struct NetworkService {
    xpc_connection_t listener;
    dispatch_queue_t queue;
    VZNetworkServiceState *state;
};

struct NetworkRequest {
    xpc_connection_t connection;
    dispatch_queue_t queue;
    dispatch_semaphore_t done;
    xpc_object_t reply;
};

static void sharingError(NSInteger code, NSString *message, void **errorOut)
{
    @autoreleasepool {
        if (errorOut)
            *errorOut = [[NSError errorWithDomain:@"com.codehex.vz.vmnet.xpc" code:code userInfo:@{NSLocalizedDescriptionKey: message}] retain];
    }
}

static int setPeerRequirement(xpc_connection_t connection, const char *requirement)
{
    if (@available(macOS 12, *))
        return xpc_connection_set_peer_code_signing_requirement(connection, requirement);
    return ENOTSUP;
}

static void discardInactiveConnection(xpc_connection_t connection)
{
    xpc_connection_set_event_handler(connection, ^(xpc_object_t event) {});
    xpc_connection_cancel(connection);
    xpc_connection_activate(connection);
    xpc_release(connection);
}

void *vz_listenVmnetNetwork(const char *name, void *serialization, const char *requirement, void **errorOut)
{
    struct NetworkService *service = calloc(1, sizeof(*service));
    if (!service) {
        sharingError(ENOMEM, @"Cannot allocate network service", errorOut);
        return NULL;
    }
    service->queue = dispatch_queue_create("com.codehex.vz.vmnet.service", DISPATCH_QUEUE_SERIAL);
    service->listener = xpc_connection_create_mach_service(name, service->queue, XPC_CONNECTION_MACH_SERVICE_LISTENER);
    int error = setPeerRequirement(service->listener, requirement);
    if (error) {
        sharingError(error, @"Invalid peer code signing requirement", errorOut);
        discardInactiveConnection(service->listener);
        dispatch_release(service->queue);
        free(service);
        return NULL;
    }
    VZNetworkServiceState *state = [[VZNetworkServiceState alloc] init];
    state->serialization = xpc_retain(serialization);
    state->peers = [[NSMutableSet alloc] init];
    service->state = state;
    dispatch_queue_t queue = service->queue;
    NSString *peerRequirement = [[NSString alloc] initWithUTF8String:requirement];
    xpc_connection_set_event_handler(service->listener, ^(xpc_object_t event) {
        if (xpc_get_type(event) != XPC_TYPE_CONNECTION)
            return;
        xpc_connection_t peer = event;
        if (state->closed || setPeerRequirement(peer, [peerRequirement UTF8String])) {
            xpc_connection_cancel(peer);
            return;
        }
        [state->peers addObject:peer];
        xpc_connection_set_target_queue(peer, queue);
        xpc_connection_set_event_handler(peer, ^(xpc_object_t message) {
            if (xpc_get_type(message) == XPC_TYPE_ERROR) {
                [state->peers removeObject:peer];
                return;
            }
            if (state->closed || xpc_get_type(message) != XPC_TYPE_DICTIONARY)
                return;
            xpc_object_t reply = xpc_dictionary_create_reply(message);
            if (!reply)
                return;
            if (xpc_dictionary_get_uint64(message, "version") == 1)
                xpc_dictionary_set_value(reply, "network", state->serialization);
            else
                xpc_dictionary_set_string(reply, "error", "Unsupported network sharing protocol");
            xpc_connection_send_message(peer, reply);
            xpc_release(reply);
        });
        xpc_connection_activate(peer);
    });
    [peerRequirement release];
    xpc_connection_activate(service->listener);
    return service;
}

void vz_closeVmnetNetworkService(void *pointer)
{
    struct NetworkService *service = pointer;
    VZNetworkServiceState *state = service->state;
    dispatch_sync(service->queue, ^{
        state->closed = true;
        xpc_connection_cancel(service->listener);
        for (id peer in state->peers)
            xpc_connection_cancel(peer);
        [state->peers removeAllObjects];
    });
    xpc_release(service->listener);
    dispatch_release(service->queue);
    [state release];
    free(service);
}

void *vz_requestVmnetNetwork(const char *name, const char *requirement, void **errorOut)
{
    struct NetworkRequest *request = calloc(1, sizeof(*request));
    if (!request) {
        sharingError(ENOMEM, @"Cannot allocate network request", errorOut);
        return NULL;
    }
    request->queue = dispatch_queue_create("com.codehex.vz.vmnet.request", DISPATCH_QUEUE_SERIAL);
    request->done = dispatch_semaphore_create(0);
    request->connection = xpc_connection_create_mach_service(name, request->queue, 0);
    int error = setPeerRequirement(request->connection, requirement);
    if (error) {
        sharingError(error, @"Invalid peer code signing requirement", errorOut);
        discardInactiveConnection(request->connection);
        dispatch_release(request->done);
        dispatch_release(request->queue);
        free(request);
        return NULL;
    }
    xpc_connection_set_event_handler(request->connection, ^(xpc_object_t event) {});
    xpc_connection_activate(request->connection);
    xpc_object_t message = xpc_dictionary_create(NULL, NULL, 0);
    xpc_dictionary_set_uint64(message, "version", 1);
    xpc_connection_send_message_with_reply(request->connection, message, request->queue, ^(xpc_object_t reply) {
        request->reply = xpc_retain(reply);
        dispatch_semaphore_signal(request->done);
    });
    xpc_release(message);
    return request;
}

void vz_cancelVmnetNetworkRequest(void *pointer)
{
    struct NetworkRequest *request = pointer;
    xpc_connection_cancel(request->connection);
}

void *vz_waitVmnetNetworkRequest(void *pointer, void **errorOut)
{
    struct NetworkRequest *request = pointer;
    dispatch_semaphore_wait(request->done, DISPATCH_TIME_FOREVER);
    dispatch_sync(request->queue, ^{});
    xpc_object_t reply = request->reply;
    if (xpc_get_type(reply) != XPC_TYPE_DICTIONARY) {
        sharingError(EIO, @"Network service request failed or peer authentication was rejected", errorOut);
        return NULL;
    }
    xpc_object_t network = xpc_dictionary_get_value(reply, "network");
    if (!network) {
        sharingError(EPROTO, @"Network service returned no serialization", errorOut);
        return NULL;
    }
    return xpc_retain(network);
}

void vz_releaseVmnetNetworkRequest(void *pointer)
{
    struct NetworkRequest *request = pointer;
    xpc_connection_cancel(request->connection);
    xpc_release(request->connection);
    if (request->reply)
        xpc_release(request->reply);
    dispatch_release(request->done);
    dispatch_release(request->queue);
    free(request);
}
