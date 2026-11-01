#import <Foundation/Foundation.h>
#import <Virtualization/Virtualization.h>
#include <errno.h>
#include <poll.h>
#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>
#include <unistd.h>

typedef struct {
    const char *name;
    dispatch_queue_t queue;
    dispatch_semaphore_t done;
    VZVirtualMachine *vm;
    int result;
} Probe;

static void trace(const char *format, ...)
{
    va_list args;
    va_start(args, format);
    vfprintf(stdout, format, args);
    fputc('\n', stdout);
    fflush(stdout);
    va_end(args);
}

static double now(void)
{
    struct timespec value;
    if (clock_gettime(CLOCK_MONOTONIC, &value) != 0) _exit(70);
    return value.tv_sec + value.tv_nsec / 1000000000.0;
}

static void protocolFailure(const char *reason)
{
    trace("PROTOCOL_FAILURE reason=%s implicit_process_teardown=1", reason);
    _exit(65);
}

static void requirePermit(unsigned sequence, long iteration, const char *phase)
{
    trace("PHASE_READY sequence=%u iteration=%ld phase=%s", sequence, iteration, phase);
    char expected[64], received[64];
    int length = snprintf(expected, sizeof(expected), "PERMIT %u", sequence);
    if (length < 0 || (size_t)length >= sizeof(expected)) protocolFailure("format");
    size_t used = 0;
    double deadline = now() + 60.0;
    for (;;) {
        double left = deadline - now();
        if (left <= 0) protocolFailure("permit_timeout");
        struct pollfd input = {.fd=STDIN_FILENO, .events=POLLIN};
        int ready = poll(&input, 1, (int)(left * 1000) + 1);
        if (ready < 0 && errno == EINTR) continue;
        if (ready < 0) protocolFailure("poll_error");
        if (ready == 0) continue;
        char byte;
        ssize_t count = read(STDIN_FILENO, &byte, 1);
        if (count < 0 && errno == EINTR) continue;
        if (count <= 0) protocolFailure("input_closed");
        if (byte == '\n') {
            if (used != (size_t)length || memcmp(received, expected, used) != 0)
                protocolFailure("permit_mismatch");
            trace("PHASE_PERMITTED sequence=%u iteration=%ld phase=%s", sequence, iteration, phase);
            return;
        }
        if (used == sizeof(received)) protocolFailure("permit_too_long");
        received[used++] = byte;
    }
}

static void complete(unsigned sequence, long iteration, const char *phase)
{
    trace("PHASE_COMPLETE sequence=%u iteration=%ld phase=%s", sequence, iteration, phase);
}

static void waitFor(Probe *probe)
{
    if (dispatch_semaphore_wait(probe->done, dispatch_time(DISPATCH_TIME_NOW, 60 * NSEC_PER_SEC)) != 0) {
        trace("ACTION_TIMEOUT vm=%s implicit_process_teardown=1", probe->name);
        _exit(124);
    }
    dispatch_sync(probe->queue, ^{});
}

static void startProbe(Probe *probe, NSString *kernel, NSString *initramfs)
{
    dispatch_async(probe->queue, ^{
        @autoreleasepool {
            VZLinuxBootLoader *loader = [[[VZLinuxBootLoader alloc] initWithKernelURL:[NSURL fileURLWithPath:kernel]] autorelease];
            loader.initialRamdiskURL = [NSURL fileURLWithPath:initramfs];
            loader.commandLine = @"console=hvc0 vsock_port=22222";
            VZVirtualMachineConfiguration *config = [[[VZVirtualMachineConfiguration alloc] init] autorelease];
            config.bootLoader = loader;
            config.CPUCount = 1;
            config.memorySize = 256ULL * 1024 * 1024;
            config.entropyDevices = @[[[[VZVirtioEntropyDeviceConfiguration alloc] init] autorelease]];
            config.memoryBalloonDevices = @[[[[VZVirtioTraditionalMemoryBalloonDeviceConfiguration alloc] init] autorelease]];
            config.networkDevices = @[];
            config.socketDevices = @[];
            config.serialPorts = @[];
            NSError *error = nil;
            if (![config validateWithError:&error]) {
                trace("ACTION_ERROR vm=%s operation=validate code=%ld", probe->name, (long)error.code);
                probe->result = 1;
                dispatch_semaphore_signal(probe->done);
                return;
            }
            probe->vm = [[VZVirtualMachine alloc] initWithConfiguration:config queue:probe->queue];
            trace("START_BEFORE vm=%s", probe->name);
            [probe->vm startWithCompletionHandler:^(NSError *startError) {
                @autoreleasepool {
                    trace("START_CALLBACK vm=%s error=%d state=%ld", probe->name, startError != nil, (long)probe->vm.state);
                    if (startError) {
                        probe->result = 1;
                        dispatch_semaphore_signal(probe->done);
                        return;
                    }
                    dispatch_after(dispatch_time(DISPATCH_TIME_NOW, 3 * NSEC_PER_SEC), probe->queue, ^{
                        @autoreleasepool {
                            probe->result = probe->vm.state == VZVirtualMachineStateRunning ? 0 : 1;
                            trace("START_READY vm=%s state=%ld result=%d", probe->name, (long)probe->vm.state, probe->result);
                            dispatch_semaphore_signal(probe->done);
                        }
                    });
                }
            }];
        }
    });
    waitFor(probe);
}

static void stopProbe(Probe *probe)
{
    dispatch_async(probe->queue, ^{
        @autoreleasepool {
            trace("STOP_BEFORE vm=%s state=%ld retained=1", probe->name, (long)probe->vm.state);
            if (!probe->vm || !probe->vm.canStop) {
                probe->result = 1;
                trace("ACTION_ERROR vm=%s operation=canStop", probe->name);
                dispatch_semaphore_signal(probe->done);
                return;
            }
            [probe->vm stopWithCompletionHandler:^(NSError *error) {
                @autoreleasepool {
                    VZVirtualMachineState state = probe->vm.state;
                    probe->result = error || state != VZVirtualMachineStateStopped ? 1 : 0;
                    trace("STOP_CALLBACK vm=%s error=%d state=%ld retained=1 result=%d", probe->name, error != nil, (long)state, probe->result);
                    dispatch_semaphore_signal(probe->done);
                }
            }];
        }
    });
    waitFor(probe);
}

static void checkState(Probe *probe, VZVirtualMachineState expected)
{
    dispatch_sync(probe->queue, ^{
        long state = probe->vm ? (long)probe->vm.state : -1;
        probe->result = probe->vm && state == (long)expected ? 0 : 1;
        trace("STATE_CHECK clock_monotonic=%.9f vm=%s state=%ld expected=%ld result=%d", now(), probe->name, state, (long)expected, probe->result);
    });
}

static void releaseProbe(Probe *probe)
{
    dispatch_sync(probe->queue, ^{
        @autoreleasepool {
            if (!probe->vm || probe->vm.state != VZVirtualMachineStateStopped) {
                probe->result = 1;
                trace("ACTION_ERROR vm=%s operation=release_precondition", probe->name);
                return;
            }
            trace("RELEASE_BEFORE vm=%s state=%ld", probe->name, (long)probe->vm.state);
            [probe->vm release];
            probe->vm = nil;
            trace("RELEASE_AFTER vm=%s", probe->name);
        }
    });
}

static long option(const char *name, long fallback, long minimum, long maximum)
{
    const char *value = getenv(name);
    if (!value) return fallback;
    char *end = NULL;
    errno = 0;
    long result = strtol(value, &end, 10);
    if (errno || end == value || *end || result < minimum || result > maximum) {
        trace("CONFIG_ERROR variable=%s", name);
        exit(2);
    }
    return result;
}

static void checkResult(Probe *probe, unsigned *sequence, long iteration)
{
    if (probe->result) {
        trace("ACTION_FAILED vm=%s iteration=%ld awaiting_abort_permission=1", probe->name, iteration);
        requirePermit(++*sequence, iteration, "abort-exit");
        trace("ABORT_EXIT implicit_process_teardown=1");
        _exit(1);
    }
}

int main(int argc, char **argv)
{
    @autoreleasepool {
        if (argc != 3) {
            fprintf(stderr, "Usage: COUNT=5 RETAIN_STOPPED=0|1 %s kernel initramfs\n", argv[0]);
            return 2;
        }
        long count = option("COUNT", 5, 1, 100);
        int retainStopped = (int)option("RETAIN_STOPPED", 0, 0, 1);
        NSString *kernel = [NSString stringWithUTF8String:argv[1]];
        NSString *initramfs = [NSString stringWithUTF8String:argv[2]];
        unsigned sequence = 0;
        trace("CONFIG pid=%d count=%ld retain_stopped=%d cpu=1 memory_mib=256 nat=0 dgram=0 socket_device=0 connect=0", getpid(), count, retainStopped);
        for (long iteration = 1; iteration <= count; iteration++) {
            Probe peer = {.name="peer", .queue=dispatch_queue_create("ci306.phase.peer", DISPATCH_QUEUE_SERIAL), .done=dispatch_semaphore_create(0)};
            Probe client = {.name="client", .queue=dispatch_queue_create("ci306.phase.client", DISPATCH_QUEUE_SERIAL), .done=dispatch_semaphore_create(0)};
            requirePermit(++sequence, iteration, "start-peer");
            startProbe(&peer, kernel, initramfs);
            checkResult(&peer, &sequence, iteration);
            complete(sequence, iteration, "start-peer");
            requirePermit(++sequence, iteration, "start-client");
            startProbe(&client, kernel, initramfs);
            checkResult(&client, &sequence, iteration);
            complete(sequence, iteration, "start-client");
            checkState(&peer, VZVirtualMachineStateRunning);
            checkState(&client, VZVirtualMachineStateRunning);
            checkResult(&peer, &sequence, iteration);
            checkResult(&client, &sequence, iteration);
            requirePermit(++sequence, iteration, "both-running");
            checkState(&peer, VZVirtualMachineStateRunning);
            checkState(&client, VZVirtualMachineStateRunning);
            checkResult(&peer, &sequence, iteration);
            checkResult(&client, &sequence, iteration);
            complete(sequence, iteration, "both-running");
            requirePermit(++sequence, iteration, "stop-client");
            stopProbe(&client);
            checkResult(&client, &sequence, iteration);
            complete(sequence, iteration, "stop-client");
            if (!retainStopped) {
                requirePermit(++sequence, iteration, "release-client");
                checkState(&peer, VZVirtualMachineStateRunning);
                if (peer.result) trace("COMPARISON_INVALID iteration=%ld boundary=release-client expected_peer=running", iteration);
                checkResult(&peer, &sequence, iteration);
                releaseProbe(&client);
                checkResult(&client, &sequence, iteration);
                complete(sequence, iteration, "release-client");
            }
            requirePermit(++sequence, iteration, "stop-peer");
            stopProbe(&peer);
            checkResult(&peer, &sequence, iteration);
            complete(sequence, iteration, "stop-peer");
            if (retainStopped) {
                requirePermit(++sequence, iteration, "release-client");
                checkState(&peer, VZVirtualMachineStateStopped);
                if (peer.result) trace("COMPARISON_INVALID iteration=%ld boundary=release-client expected_peer=stopped", iteration);
                checkResult(&peer, &sequence, iteration);
                releaseProbe(&client);
                checkResult(&client, &sequence, iteration);
                complete(sequence, iteration, "release-client");
            }
            requirePermit(++sequence, iteration, "release-peer");
            releaseProbe(&peer);
            checkResult(&peer, &sequence, iteration);
            complete(sequence, iteration, "release-peer");
            dispatch_release(client.done);
            dispatch_release(client.queue);
            dispatch_release(peer.done);
            dispatch_release(peer.queue);
            trace("CASE_COMPLETE iteration=%ld result=0", iteration);
        }
        trace("ALL_COMPLETE count=%ld result=0", count);
        return 0;
    }
}
