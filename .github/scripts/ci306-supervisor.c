#define _POSIX_C_SOURCE 200809L
#include <errno.h>
#include <fcntl.h>
#include <limits.h>
#include <poll.h>
#include <signal.h>
#include <spawn.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/types.h>
#include <sys/wait.h>
#include <time.h>
#include <unistd.h>

extern char **environ;

static int writeAll(int fd, const char *data, size_t length)
{
    while (length) {
        ssize_t written = write(fd, data, length);
        if (written < 0 && errno == EINTR) continue;
        if (written <= 0) return -1;
        data += written;
        length -= (size_t)written;
    }
    return 0;
}

static double monotonic(void)
{
    struct timespec value;
    if (clock_gettime(CLOCK_MONOTONIC, &value) != 0) {
        perror("clock_gettime monotonic");
        exit(125);
    }
    return (double)value.tv_sec + (double)value.tv_nsec / 1e9;
}

static int record(int fd, uint64_t *sequence, pid_t child, const char *event, int detail)
{
    struct timespec wall, mono;
    if (clock_gettime(CLOCK_REALTIME, &wall) || clock_gettime(CLOCK_MONOTONIC, &mono)) return -1;
    struct pollfd output = {.fd = STDOUT_FILENO, .events = POLLOUT};
    int stdoutReady = poll(&output, 1, 0) > 0 && (output.revents & POLLOUT);
    char line[512];
    int length = snprintf(line, sizeof(line),
        "VZSUP seq=%llu wall=%lld.%09ld mono=%lld.%09ld supervisor=%ld child=%ld pgid=%ld event=%s detail=%d stdout_ready=%d\n",
        (unsigned long long)++*sequence, (long long)wall.tv_sec, wall.tv_nsec,
        (long long)mono.tv_sec, mono.tv_nsec, (long)getpid(), (long)child, (long)child,
        event, detail, stdoutReady);
    if (length < 0 || (size_t)length >= sizeof(line)) { errno = EOVERFLOW; return -1; }
    if (writeAll(fd, line, (size_t)length) || fsync(fd)) return -1;
    if (stdoutReady) {
        ssize_t ignored = write(STDOUT_FILENO, line, (size_t)length);
        (void)ignored;
    }
    return 0;
}

static int groupSignal(pid_t child, int signalNumber)
{
    if (child <= 1 || child == getpgrp()) { errno = EINVAL; return -1; }
    return kill(-child, signalNumber);
}

int main(int argc, char **argv)
{
    if (argc < 4) {
        fprintf(stderr, "Usage: %s deadline-seconds journal command [args...]\n", argv[0]);
        return 2;
    }
    char *end;
    errno = 0;
    long seconds = strtol(argv[1], &end, 10);
    if (errno || end == argv[1] || *end || seconds < 1 || seconds > INT_MAX) {
        fprintf(stderr, "deadline-seconds must be 1..%d\n", INT_MAX);
        return 2;
    }
    int journal = open(argv[2], O_CREAT | O_APPEND | O_WRONLY | O_CLOEXEC, 0600);
    if (journal < 0) { perror("journal open"); return 125; }
    signal(SIGPIPE, SIG_IGN);
    posix_spawnattr_t attributes;
    int error = posix_spawnattr_init(&attributes);
    if (error) { errno = error; perror("spawnattr init"); close(journal); return 125; }
    sigset_t defaults;
    sigemptyset(&defaults);
    sigaddset(&defaults, SIGPIPE);
    error = posix_spawnattr_setpgroup(&attributes, 0);
    if (!error) error = posix_spawnattr_setsigdefault(&attributes, &defaults);
    if (!error) error = posix_spawnattr_setflags(&attributes, POSIX_SPAWN_SETPGROUP | POSIX_SPAWN_SETSIGDEF);
    pid_t child = -1;
    double start = monotonic();
    if (!error) error = posix_spawnp(&child, argv[3], NULL, &attributes, &argv[3], environ);
    posix_spawnattr_destroy(&attributes);
    if (error) { errno = error; perror("spawn child"); close(journal); return 127; }

    uint64_t sequence = 0;
    double nextTick = start;
    double termTime = 0;
    int phase = 0, failure = 0, status = 0;
    if (record(journal, &sequence, child, "spawn", (int)getpgid(child))) failure = 1;
    for (;;) {
        double now = monotonic();
        /* Keep the leader unreaped between TERM and KILL so its PID cannot be reused. */
        if (phase != 1) {
            pid_t waited = waitpid(child, &status, WNOHANG);
            if (waited == child) {
                int result = WIFEXITED(status) ? WEXITSTATUS(status) : 128 + WTERMSIG(status);
                if (record(journal, &sequence, child, "exit", result)) failure = 1;
                close(journal);
                return failure ? 125 : phase ? 124 : result;
            }
            if (waited < 0 && errno != EINTR) {
                perror("waitpid");
                close(journal);
                return 125;
            }
        }
        if (!phase && (failure || now - start >= (double)seconds)) {
            phase = 1;
            termTime = now;
            int sent = groupSignal(child, SIGTERM);
            int signalError = sent ? errno : 0;
            if (record(journal, &sequence, child, "term", signalError)) failure = 1;
        }
        if (phase == 1 && now - termTime >= 2.0) {
            int sent = groupSignal(child, SIGKILL);
            int signalError = sent ? errno : 0;
            if (record(journal, &sequence, child, "kill", signalError)) failure = 1;
            phase = 2;
        }
        if (now >= nextTick) {
            if (record(journal, &sequence, child, "tick", phase)) failure = 1;
            nextTick = now + 1.0;
        }
        struct timespec delay = {.tv_sec = 0, .tv_nsec = 100000000};
        while (nanosleep(&delay, &delay) != 0 && errno == EINTR) {}
    }
}
