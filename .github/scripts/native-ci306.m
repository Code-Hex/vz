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
    for (unsigned i = 0; i < 4; i++) {
        if (probe->sockets[i] >= 0) {
            close(probe->sockets[i]);
            probe->sockets[i] = -1;
        }
    }
    probe->result = result;
    trace("finish result=%d", result);
    dispatch_semaphore_signal(probe->done);
}

static void stop(Probe *probe, int result)
{
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

static void connectGuest(Probe *probe)
{
    trace("connect before attempt=%u port=%u", ++probe->attempts, probe->port);
    VZVirtioSocketDevice *device = (VZVirtioSocketDevice *)probe->vm.socketDevices.firstObject;
    if (!device) {
        trace("missing socket device");
        stop(probe, 1);
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
                    stop(probe, 1);
                }
                return;
            }
            probe->connection = [connection retain];
            int fd = dup(connection.fileDescriptor);
            trace("dup fd=%d errno=%d", fd, fd < 0 ? errno : 0);
            if (fd < 0) {
                stop(probe, 1);
                return;
            }
            if (probe->nonblock) {
                int flags = fcntl(fd, F_GETFL);
                if (flags < 0 || fcntl(fd, F_SETFL, flags | O_NONBLOCK) < 0) {
                    trace("nonblock failed errno=%d", errno);
                    close(fd);
                    stop(probe, 1);
                    return;
                }
                trace("nonblock set duplicate=%d original_flags=%d", fd, fcntl(connection.fileDescriptor, F_GETFL));
            }
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
                close(fd);
                trace("duplicate fd closed");
                dispatch_async(probe->queue, ^{
                    @autoreleasepool { stop(probe, result); }
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

int main(int argc, char **argv)
{
    @autoreleasepool {
        if (argc != 3) {
            fprintf(stderr, "Usage: NAT=0|1 DGRAM=0|2 CONSOLE=0|1 NONBLOCK=0|1 EARLY_RELEASE=0|1 %s kernel initramfs\n", argv[0]);
            return 2;
        }
        int nat = option("NAT", 1), datagrams = option("DGRAM", 2);
        int consoleEnabled = option("CONSOLE", 1);
        Probe probe = { .sockets = {-1, -1, -1, -1}, .port = 22222, .result = 1 };
        Probe *p = &probe;
        p->nonblock = option("NONBLOCK", 1);
        p->earlyRelease = option("EARLY_RELEASE", 1);
        p->queue = dispatch_queue_create("com.codehex.vz.native-only", DISPATCH_QUEUE_SERIAL);
        p->done = dispatch_semaphore_create(0);
        NSString *kernel = [NSString stringWithUTF8String:argv[1]];
        NSString *initramfs = [NSString stringWithUTF8String:argv[2]];
        trace("begin nat=%d dgram=%d console=%d nonblock=%d early_release=%d pid=%d",
              nat, datagrams, consoleEnabled, p->nonblock, p->earlyRelease, getpid());
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
                    if (consoleFD < 0) { trace("console dup errno=%d", errno); finish(p, 1); return; }
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
                    if (socketpair(AF_UNIX, SOCK_DGRAM, 0, &p->sockets[i * 2]) < 0) {
                        trace("socketpair errno=%d", errno); finish(p, 1); return;
                    }
                    int fd = dup(p->sockets[i * 2]);
                    if (fd < 0) { trace("datagram dup errno=%d", errno); finish(p, 1); return; }
                    NSFileHandle *handle = [[[NSFileHandle alloc] initWithFileDescriptor:fd closeOnDealloc:YES] autorelease];
                    VZVirtioNetworkDeviceConfiguration *nic = [[[VZVirtioNetworkDeviceConfiguration alloc] init] autorelease];
                    nic.attachment = [[[VZFileHandleNetworkDeviceAttachment alloc] initWithFileHandle:handle] autorelease];
                    nic.MACAddress = [VZMACAddress randomLocallyAdministeredAddress];
                    [networks addObject:nic];
                    trace("datagram created index=%d original=%d peer=%d duplicate=%d", i, p->sockets[i * 2], p->sockets[i * 2 + 1], fd);
                }
                config.networkDevices = networks;
                NSError *error = nil;
                if (![config validateWithError:&error]) {
                    trace("validation failed error=%s", error.description.UTF8String); finish(p, 1); return;
                }
                trace("vm create before");
                p->vm = [[VZVirtualMachine alloc] initWithConfiguration:config queue:p->queue];
                trace("start before");
                [p->vm startWithCompletionHandler:^(NSError *error) {
                    @autoreleasepool {
                        trace("start callback error=%s", error ? error.description.UTF8String : "nil");
                        if (error) { finish(p, 1); return; }
                        dispatch_after(dispatch_time(DISPATCH_TIME_NOW, 3 * NSEC_PER_SEC), p->queue, ^{
                            @autoreleasepool { connectGuest(p); }
                        });
                    }
                }];
                trace("start returned");
            }
        });
        if (dispatch_semaphore_wait(p->done, dispatch_time(DISPATCH_TIME_NOW, 120 * NSEC_PER_SEC)) != 0) {
            trace("outer timeout");
            _exit(124);
        }
        dispatch_sync(p->queue, ^{});
        dispatch_release(p->done);
        dispatch_release(p->queue);
        return p->result;
    }
}
