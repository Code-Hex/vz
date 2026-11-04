#import <Foundation/Foundation.h>
#import <Virtualization/Virtualization.h>
#import <vmnet/vmnet.h>
#include <dispatch/dispatch.h>
#include <arpa/inet.h>
#include <errno.h>
#include <fcntl.h>
#include <net/ethernet.h>
#include <poll.h>
#include <signal.h>
#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>
#include <unistd.h>

@interface Guest : NSObject {
@public
    dispatch_queue_t queue;
    VZVirtualMachineConfiguration *configuration;
    VZVirtualMachine *machine;
    VZVirtioSocketConnection *connection;
    int descriptor;
    int index;
}
@end
@implementation Guest
@end

static void stage(const char *format, ...)
{
    struct timespec now;
    struct tm utc;
    char date[32], message[1024];
    clock_gettime(CLOCK_REALTIME, &now);
    gmtime_r(&now.tv_sec, &utc);
    strftime(date, sizeof(date), "%Y-%m-%dT%H:%M:%S", &utc);
    va_list args;
    va_start(args, format);
    vsnprintf(message, sizeof(message), format, args);
    va_end(args);
    fprintf(stderr, "%s.%09ldZ pid=%d %s\n", date, now.tv_nsec, getpid(), message);
    fflush(stderr);
}

static double monotonic(void)
{
    struct timespec now;
    clock_gettime(CLOCK_MONOTONIC, &now);
    return now.tv_sec + now.tv_nsec / 1e9;
}

static void await(dispatch_semaphore_t done, double deadline, const char *operation)
{
    double remaining = deadline - monotonic();
    if (remaining <= 0 || dispatch_semaphore_wait(done,
            dispatch_time(DISPATCH_TIME_NOW, (int64_t)(remaining * NSEC_PER_SEC))) != 0) {
        stage("%s timeout; exiting without blocking native teardown", operation);
        _Exit(124);
    }
}

static void status(const char *operation, vmnet_return_t value)
{
    stage("%s exit status=%u", operation, (unsigned)value);
    if (value != VMNET_SUCCESS)
        _Exit(1);
}

static BOOL startGuest(Guest *guest, NSURL *kernel, NSURL *initrd,
    vmnet_network_ref network, double deadline)
{
    dispatch_semaphore_t done = dispatch_semaphore_create(0);
    __block BOOL success = NO;
    dispatch_async(guest->queue, ^{
        @autoreleasepool {
            stage("guest=%d configuration enter", guest->index);
            VZLinuxBootLoader *boot = [[VZLinuxBootLoader alloc] initWithKernelURL:kernel];
            boot.initialRamdiskURL = initrd;
            boot.commandLine = @"console=hvc0 vsock_port=32022";
            guest->configuration = [[VZVirtualMachineConfiguration alloc] init];
            guest->configuration.bootLoader = boot;
            guest->configuration.CPUCount = 1;
            guest->configuration.memorySize = 256 * 1024 * 1024;
            guest->configuration.entropyDevices = @[[[VZVirtioEntropyDeviceConfiguration alloc] init]];
            guest->configuration.memoryBalloonDevices = @[[[VZVirtioTraditionalMemoryBalloonDeviceConfiguration alloc] init]];
            guest->configuration.socketDevices = @[[[VZVirtioSocketDeviceConfiguration alloc] init]];
            VZVirtioConsoleDeviceSerialPortConfiguration *console = [[VZVirtioConsoleDeviceSerialPortConfiguration alloc] init];
            console.attachment = [[VZFileHandleSerialPortAttachment alloc]
                initWithFileHandleForReading:nil fileHandleForWriting:NSFileHandle.fileHandleWithStandardError];
            guest->configuration.serialPorts = @[console];
            VZVirtioNetworkDeviceConfiguration *device = [[VZVirtioNetworkDeviceConfiguration alloc] init];
            device.attachment = [[VZVmnetNetworkDeviceAttachment alloc] initWithNetwork:network];
            device.MACAddress = [[VZMACAddress alloc] initWithString:
                [NSString stringWithFormat:@"02:00:00:00:00:%02x", 10 + guest->index]];
            guest->configuration.networkDevices = @[device];
            stage("guest=%d configuration exit", guest->index);
            NSError *error = nil;
            stage("guest=%d validate enter", guest->index);
            BOOL valid = [guest->configuration validateWithError:&error];
            stage("guest=%d validate exit valid=%d error=%s", guest->index, valid,
                error ? error.description.UTF8String : "nil");
            if (!valid) {
                dispatch_semaphore_signal(done);
                return;
            }
            stage("guest=%d VM init enter", guest->index);
            guest->machine = [[VZVirtualMachine alloc] initWithConfiguration:guest->configuration queue:guest->queue];
            stage("guest=%d VM init exit", guest->index);
            if (!guest->machine || !guest->machine.canStart) {
                stage("guest=%d cannot start", guest->index);
                dispatch_semaphore_signal(done);
                return;
            }
            stage("guest=%d start call enter", guest->index);
            [guest->machine startWithCompletionHandler:^(NSError *error) {
                success = !error && guest->machine.state == VZVirtualMachineStateRunning;
                stage("guest=%d start completion state=%ld error=%s", guest->index,
                    (long)guest->machine.state, error ? error.description.UTF8String : "nil");
                dispatch_semaphore_signal(done);
            }];
            stage("guest=%d start call exit", guest->index);
        }
    });
    await(done, deadline, "guest start");
    return success;
}

static BOOL banner(Guest *guest, double deadline)
{
    int fd = guest->descriptor;
    int flags = fcntl(fd, F_GETFL);
    if (flags < 0 || fcntl(fd, F_SETFL, flags | O_NONBLOCK) < 0)
        return NO;
    const char identification[] = "SSH-2.0-vz-native-probe\r\n";
    size_t sent = 0, used = 0;
    char text[512] = {0};
    stage("guest=%d SSH identification/banner enter", guest->index);
    while (monotonic() < deadline) {
        int milliseconds = (int)((deadline - monotonic()) * 1000);
        if (milliseconds < 1)
            break;
        struct pollfd item = {.fd = fd, .events = sent < sizeof(identification) - 1 ? POLLOUT : POLLIN};
        int ready = poll(&item, 1, milliseconds);
        if (ready < 0 && errno == EINTR)
            continue;
        if (ready <= 0)
            break;
        ssize_t count;
        if (sent < sizeof(identification) - 1) {
            count = write(fd, identification + sent, sizeof(identification) - 1 - sent);
            if (count > 0)
                sent += (size_t)count;
        } else {
            count = read(fd, text + used, sizeof(text) - 1 - used);
            if (count > 0) {
                used += (size_t)count;
                char *version = strstr(text, "SSH-");
                if (version && strchr(version, '\n')) {
                    stage("guest=%d SSH identification/banner exit version=%.*s", guest->index,
                        (int)strcspn(version, "\r\n"), version);
                    return YES;
                }
                if (used == sizeof(text) - 1)
                    break;
            }
        }
        if (count == 0 || (count < 0 && errno != EINTR && errno != EAGAIN && errno != EWOULDBLOCK))
            break;
    }
    stage("guest=%d SSH identification/banner exit failed errno=%d", guest->index, errno);
    return NO;
}

static BOOL connectGuest(Guest *guest, double deadline)
{
    for (int attempt = 1; monotonic() < deadline; attempt++) {
        dispatch_semaphore_t done = dispatch_semaphore_create(0);
        __block NSError *resultError = nil;
        dispatch_async(guest->queue, ^{
            stage("guest=%d vsock enter attempt=%d", guest->index, attempt);
            VZVirtioSocketDevice *socket = (VZVirtioSocketDevice *)guest->machine.socketDevices.firstObject;
            if (!socket) {
                resultError = [NSError errorWithDomain:NSPOSIXErrorDomain code:ENODEV userInfo:nil];
                dispatch_semaphore_signal(done);
                return;
            }
            [socket connectToPort:32022 completionHandler:^(VZVirtioSocketConnection *connection, NSError *error) {
                resultError = error;
                if (connection && !error) {
                    guest->connection = connection;
                    guest->descriptor = dup(connection.fileDescriptor);
                    if (guest->descriptor < 0)
                        resultError = [NSError errorWithDomain:NSPOSIXErrorDomain code:errno userInfo:nil];
                } else if (!error) {
                    resultError = [NSError errorWithDomain:NSPOSIXErrorDomain code:ENOTCONN userInfo:nil];
                }
                stage("guest=%d vsock completion attempt=%d fd=%d error=%s", guest->index, attempt,
                    guest->descriptor, resultError ? resultError.description.UTF8String : "nil");
                dispatch_semaphore_signal(done);
            }];
            stage("guest=%d vsock call exit attempt=%d", guest->index, attempt);
        });
        await(done, deadline, "vsock completion");
        if (!resultError)
            return banner(guest, deadline);
        if (![resultError.domain isEqualToString:NSPOSIXErrorDomain] || resultError.code != ECONNRESET)
            return NO;
        usleep(200000);
    }
    stage("guest=%d connection deadline expired", guest->index);
    return NO;
}

static BOOL stopGuest(Guest *guest)
{
    dispatch_semaphore_t done = dispatch_semaphore_create(0);
    __block BOOL success = YES;
    dispatch_async(guest->queue, ^{
        void (^releaseGuest)(void) = ^{
            if (guest->descriptor >= 0)
                close(guest->descriptor);
            guest->descriptor = -1;
            [guest->connection close];
            guest->connection = nil;
            guest->machine = nil;
            guest->configuration = nil;
            stage("guest=%d teardown exit", guest->index);
            dispatch_semaphore_signal(done);
        };
        stage("guest=%d stop enter", guest->index);
        if (!guest->machine || guest->machine.state == VZVirtualMachineStateStopped ||
            guest->machine.state == VZVirtualMachineStateError) {
            dispatch_async(guest->queue, releaseGuest);
            return;
        }
        if (!guest->machine.canStop) {
            stage("guest=%d stop exit canStop=false state=%ld", guest->index, (long)guest->machine.state);
            success = NO;
            dispatch_semaphore_signal(done);
            return;
        }
        [guest->machine stopWithCompletionHandler:^(NSError *error) {
            stage("guest=%d stop completion state=%ld error=%s", guest->index,
                (long)guest->machine.state, error ? error.description.UTF8String : "nil");
            if (error) {
                success = NO;
                dispatch_semaphore_signal(done);
                return;
            }
            dispatch_async(guest->queue, releaseGuest);
        }];
        stage("guest=%d stop call exit", guest->index);
    });
    await(done, monotonic() + 10, "guest stop/teardown");
    return success;
}

int main(int argc, const char *argv[])
{
    @autoreleasepool {
        if (argc != 3) {
            fprintf(stderr, "usage: %s /path/to/Image /path/to/initramfs.cpio.gz\n", argv[0]);
            return 64;
        }
        if (@available(macOS 26.0, *)) {
            signal(SIGPIPE, SIG_IGN);
            stage("native probe enter; no Go runtime, no sample, no SSH authentication");
            vmnet_return_t result;
            stage("network configuration create enter");
            vmnet_network_configuration_ref config = vmnet_network_configuration_create(VMNET_HOST_MODE, &result);
            status("network configuration create", result);
            if (!config)
                _Exit(1);
            struct in_addr subnet, mask;
            inet_pton(AF_INET, "192.168.247.0", &subnet);
            inet_pton(AF_INET, "255.255.255.0", &mask);
            stage("network subnet/MTU/reservations enter");
            status("network subnet", vmnet_network_configuration_set_ipv4_subnet(config, &subnet, &mask));
            status("network MTU", vmnet_network_configuration_set_mtu(config, 1500));
            for (int i = 0; i < 2; i++) {
                ether_addr_t mac = {{2, 0, 0, 0, 0, (uint8_t)(10 + i)}};
                struct in_addr address;
                inet_pton(AF_INET, i ? "192.168.247.11" : "192.168.247.10", &address);
                status("network DHCP reservation", vmnet_network_configuration_add_dhcp_reservation(config, &mac, &address));
            }
            stage("network create enter");
            vmnet_network_ref network = vmnet_network_create(config, &result);
            status("network create", result);
            if (!network)
                _Exit(1);
            stage("network subnet query enter");
            vmnet_network_get_ipv4_subnet(network, &subnet, &mask);
            char actualSubnet[INET_ADDRSTRLEN], actualMask[INET_ADDRSTRLEN];
            stage("network subnet query exit subnet=%s mask=%s", inet_ntop(AF_INET, &subnet, actualSubnet, sizeof(actualSubnet)),
                inet_ntop(AF_INET, &mask, actualMask, sizeof(actualMask)));
            Guest *guests[2] = {[Guest new], [Guest new]};
            for (int i = 0; i < 2; i++) {
                guests[i]->index = i;
                guests[i]->descriptor = -1;
                guests[i]->queue = dispatch_queue_create(i ? "vz.native-probe.guest1" : "vz.native-probe.guest0", DISPATCH_QUEUE_SERIAL);
            }
            BOOL success = YES;
            NSURL *kernel = [NSURL fileURLWithPath:[NSString stringWithUTF8String:argv[1]]];
            NSURL *initrd = [NSURL fileURLWithPath:[NSString stringWithUTF8String:argv[2]]];
            for (int i = 0; i < 2; i++) {
                double deadline = monotonic() + 30;
                if (!startGuest(guests[i], kernel, initrd, network, deadline) || !connectGuest(guests[i], deadline)) {
                    success = NO;
                    break;
                }
            }
            for (int i = 1; i >= 0; i--) {
                if (!stopGuest(guests[i]))
                    _Exit(1);
            }
            stage("network release enter");
            CFRelease(network);
            CFRelease(config);
            stage("network release exit");
            stage("native probe exit success=%d", success);
            return success ? 0 : 1;
        }
        stage("macOS 26 or newer required");
        return 77;
    }
}
