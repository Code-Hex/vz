#import <Foundation/Foundation.h>
#import <Virtualization/Virtualization.h>
#include <errno.h>
#include <fcntl.h>
#include <poll.h>
#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/socket.h>
#include <time.h>
#include <unistd.h>

typedef struct {
    dispatch_queue_t queue;
    dispatch_semaphore_t done;
    dispatch_semaphore_t ready;
    const char *name;
    int bannerFD;
    VZVirtualMachine *vm;
    VZVirtioSocketConnection *connection;
    int sockets[4];
    unsigned attempts;
    uint32_t port;
    int nonblock;
    int earlyRelease;
    int result;
} Probe;

static void trace(const char *format, ...)
{
    va_list args;
    va_start(args, format);
    fprintf(stderr, "VZTRACE native-only ");
    vfprintf(stderr, format, args);
    fputc('\n', stderr);
    fflush(stderr);
    va_end(args);
}

static void finish(Probe *probe, int result)
{
    trace("release vm before result=%d", result);
    [probe->vm release];
    probe->vm = nil;
    probe->result = result;
    trace("finish result=%d", result);
    dispatch_semaphore_signal(probe->done);
}

static void stop(Probe *probe, int result)
{
    trace("%s stop requested", probe->name);
    if (probe->bannerFD >= 0) {
        close(probe->bannerFD);
        probe->bannerFD = -1;
        trace("%s banner fd closed", probe->name);
    }
    if (probe->connection) {
        trace("connection close before connection=%p", probe->connection);
        [probe->connection close];
        trace("connection release before connection=%p", probe->connection);
        [probe->connection release];
        probe->connection = nil;
        trace("connection release after");
    }
    trace("stop before state=%ld", (long)probe->vm.state);
    if (probe->vm.state == VZVirtualMachineStateStopped) {
        finish(probe, result);
        return;
    }
    if (!probe->vm.canStop) {
        trace("cannot stop state=%ld", (long)probe->vm.state);
        finish(probe, 1);
        return;
    }
    [probe->vm stopWithCompletionHandler:^(NSError *error) {
        @autoreleasepool {
            trace("stop callback error=%s", error ? error.description.UTF8String : "nil");
            finish(probe, error ? 1 : result);
        }
    }];
    trace("stop returned");
}

static int readBanner(int fd)
{
    char buffer[1024];
    size_t used = 0;
    struct timespec started;
    clock_gettime(CLOCK_MONOTONIC, &started);
    for (;;) {
        struct timespec now;
        clock_gettime(CLOCK_MONOTONIC, &now);
        long elapsed = (now.tv_sec - started.tv_sec) * 1000
            + (now.tv_nsec - started.tv_nsec) / 1000000;
        if (elapsed >= 15000) {
            trace("banner timeout");
            return 1;
        }
        struct pollfd event = { .fd = fd, .events = POLLIN };
        int ready = poll(&event, 1, (int)(15000 - elapsed));
        if (ready < 0 && errno == EINTR)
            continue;
        if (ready <= 0) {
            trace("banner poll ready=%d errno=%d", ready, ready < 0 ? errno : 0);
            return 1;
        }
        ssize_t count = read(fd, buffer + used, sizeof(buffer) - used - 1);
        if (count < 0 && (errno == EINTR || errno == EAGAIN || errno == EWOULDBLOCK))
            continue;
        if (count <= 0) {
            trace("banner read count=%zd errno=%d events=%d", count, count < 0 ? errno : 0, event.revents);
            return 1;
        }
        used += (size_t)count;
        buffer[used] = '\0';
        if (memchr(buffer, '\n', used)) {
            trace("banner received bytes=%zu ssh=%d", used, strncmp(buffer, "SSH-", 4) == 0);
            return strncmp(buffer, "SSH-", 4) != 0;
        }
        if (used == sizeof(buffer) - 1) {
            trace("banner too long");
            return 1;
        }
    }
}

static void ready(Probe *probe, int result)
{
    probe->result = result;
    trace("%s ready result=%d; vm retained", probe->name, result);
    dispatch_semaphore_signal(probe->ready);
}

static void connectGuest(Probe *probe)
{
    trace("connect before attempt=%u port=%u", ++probe->attempts, probe->port);
    VZVirtioSocketDevice *device = (VZVirtioSocketDevice *)probe->vm.socketDevices.firstObject;
    if (!device) {
        trace("missing socket device");
        ready(probe, 1);
        return;
    }
    [device connectToPort:probe->port completionHandler:^(VZVirtioSocketConnection *connection, NSError *error) {
        @autoreleasepool {
            trace("connect callback attempt=%u connection=%p error=%s", probe->attempts, connection,
                  error ? error.description.UTF8String : "nil");
            if (error || !connection) {
                if (probe->attempts < 5) {
                    dispatch_after(dispatch_time(DISPATCH_TIME_NOW, NSEC_PER_SEC), probe->queue, ^{
                        @autoreleasepool { connectGuest(probe); }
                    });
                } else {
                    ready(probe, 1);
                }
                return;
            }
            probe->connection = [connection retain];
            int fd = dup(connection.fileDescriptor);
            trace("dup fd=%d errno=%d", fd, fd < 0 ? errno : 0);
            if (fd < 0) {
                ready(probe, 1);
                return;
            }
            if (probe->nonblock) {
                int flags = fcntl(fd, F_GETFL);
                if (flags < 0 || fcntl(fd, F_SETFL, flags | O_NONBLOCK) < 0) {
                    trace("nonblock failed errno=%d", errno);
                    close(fd);
                    ready(probe, 1);
                    return;
                }
                trace("nonblock set duplicate=%d original_flags=%d", fd, fcntl(connection.fileDescriptor, F_GETFL));
            }
            probe->bannerFD = fd;
            if (probe->earlyRelease) {
                void *owned = probe->connection;
                probe->connection = nil;
                trace("early release enqueue connection=%p", owned);
                dispatch_async(probe->queue, ^{
                    @autoreleasepool {
                        trace("early release before connection=%p", owned);
                        [(VZVirtioSocketConnection *)owned release];
                        trace("early release after connection=%p", owned);
                    }
                });
            }
            dispatch_async(dispatch_get_global_queue(QOS_CLASS_DEFAULT, 0), ^{
                int result = readBanner(fd);
                trace("%s banner read complete; duplicate retained", probe->name);
                dispatch_async(probe->queue, ^{
                    @autoreleasepool { ready(probe, result); }
                });
            });
        }
    }];
    trace("connect returned");
}

static int option(const char *name, int allowed)
{
    const char *value = getenv(name);
    if (!value || strcmp(value, "0") == 0)
        return 0;
    if ((allowed == 1 && strcmp(value, "1") == 0) || (allowed == 2 && strcmp(value, "2") == 0))
        return allowed;
    fprintf(stderr, "VZTRACE native-only invalid %s: expected 0 or %d\n", name, allowed);
    exit(2);
}

static void launch(Probe *p, NSString *kernel, NSString *initramfs, int nat, int datagrams, int consoleEnabled)
{
        dispatch_async(p->queue, ^{
            @autoreleasepool {
                VZLinuxBootLoader *loader = [[[VZLinuxBootLoader alloc] initWithKernelURL:[NSURL fileURLWithPath:kernel]] autorelease];
                loader.initialRamdiskURL = [NSURL fileURLWithPath:initramfs];
                loader.commandLine = [NSString stringWithFormat:@"console=hvc0 vsock_port=%u", p->port];
                VZVirtualMachineConfiguration *config = [[[VZVirtualMachineConfiguration alloc] init] autorelease];
                config.bootLoader = loader;
                config.CPUCount = 1;
                config.memorySize = 256ULL * 1024 * 1024;
                config.entropyDevices = @[[[[VZVirtioEntropyDeviceConfiguration alloc] init] autorelease]];
                config.memoryBalloonDevices = @[[[[VZVirtioTraditionalMemoryBalloonDeviceConfiguration alloc] init] autorelease]];
                config.socketDevices = @[[[[VZVirtioSocketDeviceConfiguration alloc] init] autorelease]];
                if (consoleEnabled) {
                    int consoleFD = dup(STDERR_FILENO);
                    if (consoleFD < 0) { trace("console dup errno=%d", errno); ready(p, 1); return; }
                    NSFileHandle *console = [[[NSFileHandle alloc] initWithFileDescriptor:consoleFD closeOnDealloc:YES] autorelease];
                    VZVirtioConsoleDeviceSerialPortConfiguration *serial = [[[VZVirtioConsoleDeviceSerialPortConfiguration alloc] init] autorelease];
                    serial.attachment = [[[VZFileHandleSerialPortAttachment alloc] initWithFileHandleForReading:nil fileHandleForWriting:console] autorelease];
                    config.serialPorts = @[serial];
                }
                NSMutableArray *networks = [NSMutableArray array];
                if (nat) {
                    trace("nat create before");
                    VZVirtioNetworkDeviceConfiguration *nic = [[[VZVirtioNetworkDeviceConfiguration alloc] init] autorelease];
                    nic.attachment = [[[VZNATNetworkDeviceAttachment alloc] init] autorelease];
                    nic.MACAddress = [VZMACAddress randomLocallyAdministeredAddress];
                    [networks addObject:nic];
                    trace("nat create after");
                }
                for (int i = 0; i < datagrams; i++) {
                    int fd = dup(p->sockets[i]);
                    if (fd < 0) { trace("datagram dup errno=%d", errno); ready(p, 1); return; }
                    NSFileHandle *handle = [[[NSFileHandle alloc] initWithFileDescriptor:fd closeOnDealloc:YES] autorelease];
                    VZVirtioNetworkDeviceConfiguration *nic = [[[VZVirtioNetworkDeviceConfiguration alloc] init] autorelease];
                    nic.attachment = [[[VZFileHandleNetworkDeviceAttachment alloc] initWithFileHandle:handle] autorelease];
                    nic.MACAddress = [VZMACAddress randomLocallyAdministeredAddress];
                    [networks addObject:nic];
                    trace("%s datagram index=%d original=%d duplicate=%d", p->name, i, p->sockets[i], fd);
                }
                config.networkDevices = networks;
                NSError *error = nil;
                if (![config validateWithError:&error]) {
                    trace("validation failed error=%s", error.description.UTF8String); ready(p, 1); return;
                }
                trace("vm create before");
                p->vm = [[VZVirtualMachine alloc] initWithConfiguration:config queue:p->queue];
                trace("%s start before", p->name);
                [p->vm startWithCompletionHandler:^(NSError *error) {
                    @autoreleasepool {
                        trace("%s start callback error=%s", p->name, error ? error.description.UTF8String : "nil");
                        if (error) { ready(p, 1); return; }
                        dispatch_after(dispatch_time(DISPATCH_TIME_NOW, 3 * NSEC_PER_SEC), p->queue, ^{
                            @autoreleasepool { connectGuest(p); }
                        });
                    }
                }];
                trace("start returned");
            }
        });
}

static void waitFor(Probe *p, dispatch_semaphore_t semaphore, const char *phase)
{
    if (dispatch_semaphore_wait(semaphore, dispatch_time(DISPATCH_TIME_NOW, 120 * NSEC_PER_SEC)) != 0) {
        trace("%s timeout phase=%s", p->name, phase);
        _exit(124);
    }
    dispatch_sync(p->queue, ^{});
}

static void shutdownProbe(Probe *p)
{
    dispatch_async(p->queue, ^{
        @autoreleasepool { stop(p, p->result); }
    });
    waitFor(p, p->done, "stop");
}

int main(int argc, char **argv)
{
    @autoreleasepool {
        if (argc != 3) {
            fprintf(stderr, "Usage: NAT=0|1 DGRAM=0|2 COUNT=5 CONSOLE=0|1 NONBLOCK=0|1 EARLY_RELEASE=0|1 %s kernel initramfs\n", argv[0]);
            return 2;
        }
        int nat = option("NAT", 1), datagrams = option("DGRAM", 2);
        int consoleEnabled = option("CONSOLE", 1);
        int nonblock = option("NONBLOCK", 1), earlyRelease = option("EARLY_RELEASE", 1);
        long count = 5;
        const char *value = getenv("COUNT");
        if (value) {
            char *end = NULL;
            errno = 0;
            count = strtol(value, &end, 10);
            if (errno || end == value || *end || count < 1 || count > 100) {
                trace("COUNT must be 1..100"); return 2;
            }
        }
        NSString *kernel = [NSString stringWithUTF8String:argv[1]];
        NSString *initramfs = [NSString stringWithUTF8String:argv[2]];
        trace("pair begin nat=%d dgram=%d count=%ld console=%d nonblock=%d early_release=%d pid=%d",
              nat, datagrams, count, consoleEnabled, nonblock, earlyRelease, getpid());
        for (long iteration = 1; iteration <= count; iteration++) {
            @autoreleasepool {
                int fabric[4] = {-1, -1, -1, -1};
                for (int i = 0; i < datagrams; i++) {
                    if (socketpair(AF_UNIX, SOCK_DGRAM, 0, &fabric[i * 2]) < 0) {
                        trace("fabric socketpair errno=%d", errno);
                        for (int j = 0; j < 4; j++) if (fabric[j] >= 0) close(fabric[j]);
                        return 1;
                    }
                }
                Probe peer = {.name="peer", .sockets={fabric[1],fabric[3],-1,-1}, .port=22222,
                              .bannerFD=-1, .nonblock=nonblock, .earlyRelease=earlyRelease, .result=1};
                Probe client = {.name="client", .sockets={fabric[0],-1,-1,-1}, .port=22222,
                                .bannerFD=-1, .nonblock=nonblock, .earlyRelease=earlyRelease, .result=1};
                peer.queue = dispatch_queue_create("com.codehex.vz.native-pair.peer", DISPATCH_QUEUE_SERIAL);
                client.queue = dispatch_queue_create("com.codehex.vz.native-pair.client", DISPATCH_QUEUE_SERIAL);
                peer.done = dispatch_semaphore_create(0);
                peer.ready = dispatch_semaphore_create(0);
                client.done = dispatch_semaphore_create(0);
                client.ready = dispatch_semaphore_create(0);
                trace("case=%ld peer launch", iteration);
                launch(&peer, kernel, initramfs, nat, datagrams, consoleEnabled);
                waitFor(&peer, peer.ready, "banner");
                int result = peer.result;
                if (!result) {
                    trace("case=%ld client launch while peer retained", iteration);
                    launch(&client, kernel, initramfs, nat, datagrams ? 1 : 0, consoleEnabled);
                    waitFor(&client, client.ready, "banner");
                    result |= client.result;
                    trace("case=%ld both retained; client shutdown", iteration);
                    shutdownProbe(&client);
                    result |= client.result;
                }
                trace("case=%ld peer shutdown", iteration);
                shutdownProbe(&peer);
                result |= peer.result;
                for (int i = 0; i < 4; i++) if (fabric[i] >= 0) close(fabric[i]);
                dispatch_release(client.ready);
                dispatch_release(client.done);
                dispatch_release(client.queue);
                dispatch_release(peer.ready);
                dispatch_release(peer.done);
                dispatch_release(peer.queue);
                trace("case=%ld complete result=%d", iteration, result);
                if (result) return result;
            }
        }
        trace("pair all complete count=%ld", count);
        return 0;
    }
}
