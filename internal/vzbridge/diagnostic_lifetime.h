#pragma once

#include <errno.h>
#include <fcntl.h>
#include <pthread.h>
#include <stdatomic.h>
#include <stdint.h>
#include <stdio.h>
#include <string.h>
#include <time.h>
#include <unistd.h>

static void vzDiagnosticLifetime(const char *event, const void *object)
{
    int savedErrno = errno;
    // Each translation unit writes at most about 1 MiB.
    static _Atomic unsigned records;
    unsigned record = atomic_fetch_add(&records, 1);
    if (record > 4096)
        return;
    int fd = open("/tmp/vz-316-native-lifetime.log", O_WRONLY | O_CREAT | O_APPEND, 0600);
    if (fd >= 0) {
        struct timespec now = {0};
        uint64_t thread = 0;
        clock_gettime(CLOCK_REALTIME, &now);
        pthread_threadid_np(NULL, &thread);
        const char *scope = strrchr(__BASE_FILE__, '/');
        scope = scope ? scope + 1 : __BASE_FILE__;
        char line[256];
        int count = snprintf(line, sizeof(line), "%lld.%09ld pid=%d thread=%llu scope=%s event=%s pointer=%p\n",
            (long long)now.tv_sec, now.tv_nsec, getpid(), (unsigned long long)thread,
            scope, record == 4096 ? "trace.capped" : event, object);
        if (count > 0)
            write(fd, line, (size_t)count < sizeof(line) ? (size_t)count : sizeof(line) - 1);
        close(fd);
    }
    errno = savedErrno;
}

