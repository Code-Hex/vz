#include <dlfcn.h>
#include <errno.h>
#include <fcntl.h>
#include <stdint.h>
#include <stdio.h>
#include <string.h>
#include <sys/stat.h>
#include <time.h>
#include <unistd.h>

/* Opaque libsyscall SPI; flags match Apple XNU osfmk/kern/debug.h. */
static const uint64_t capture_flags = 0x10000ULL | 0x02ULL | 0x08ULL | 0x1000000ULL;

static double now(void) {
    struct timespec t;
    if (clock_gettime(CLOCK_MONOTONIC_RAW, &t) != 0) return -1;
    return (double)t.tv_sec + (double)t.tv_nsec / 1000000000.0;
}

int main(int argc, char **argv) {
    if (argc != 2) {
        fputs("usage: stackshot-once --symbols | NEW_OUTPUT_PATH\n", stderr);
        return 64;
    }
    void *(*create)(void) = (void *(*)(void))dlsym(RTLD_DEFAULT, "stackshot_config_create");
    int (*set_pid)(void *, int) = (int (*)(void *, int))dlsym(RTLD_DEFAULT, "stackshot_config_set_pid");
    int (*set_flags)(void *, uint64_t) = (int (*)(void *, uint64_t))dlsym(RTLD_DEFAULT, "stackshot_config_set_flags");
    int (*capture)(void *) = (int (*)(void *))dlsym(RTLD_DEFAULT, "stackshot_capture_with_config");
    void *(*get_buffer)(void *) = (void *(*)(void *))dlsym(RTLD_DEFAULT, "stackshot_config_get_stackshot_buffer");
    uint32_t (*get_size)(void *) = (uint32_t (*)(void *))dlsym(RTLD_DEFAULT, "stackshot_config_get_stackshot_size");
    int (*dealloc)(void *) = (int (*)(void *))dlsym(RTLD_DEFAULT, "stackshot_config_dealloc");
    if (!create || !set_pid || !set_flags || !capture || !get_buffer || !get_size || !dealloc) {
        puts("{\"status\":\"missing_spi\"}");
        return 69;
    }
    if (strcmp(argv[1], "--symbols") == 0) {
        puts("{\"status\":\"available\",\"symbols\":7}");
        return 0;
    }
    umask(077);
    int fd = open(argv[1], O_WRONLY | O_CREAT | O_EXCL | O_NOFOLLOW | O_CLOEXEC, 0600);
    if (fd < 0) {
        printf("{\"status\":\"open_failed\",\"errno\":%d}\n", errno);
        return 73;
    }
    void *config = create();
    int setup_error = config ? set_pid(config, -1) : ENOMEM;
    if (setup_error == 0) setup_error = set_flags(config, capture_flags);
    if (setup_error != 0) {
        if (config) dealloc(config);
        close(fd);
        unlink(argv[1]);
        printf("{\"status\":\"setup_failed\",\"errno\":%d}\n", setup_error);
        return 70;
    }
    double start = now();
    int capture_error = capture(config);
    double finish = now();
    void *buffer = capture_error == 0 ? get_buffer(config) : NULL;
    uint32_t size = capture_error == 0 ? get_size(config) : 0;
    int io_error = 0;
    if (capture_error == 0 && (!buffer || size == 0)) io_error = EIO;
    size_t offset = 0;
    while (capture_error == 0 && io_error == 0 && offset < size) {
        ssize_t count = write(fd, (const char *)buffer + offset, size - offset);
        if (count < 0 && errno == EINTR) continue;
        if (count <= 0) { io_error = count < 0 ? errno : EIO; break; }
        offset += (size_t)count;
    }
    dealloc(config);
    if (close(fd) != 0 && io_error == 0) io_error = errno;
    if (capture_error || io_error) unlink(argv[1]);
    printf("{\"status\":\"%s\",\"euid\":%u,\"flags\":%llu,\"capture_errno\":%d,\"io_errno\":%d,\"bytes\":%u,\"capture_seconds\":%.9f}\n",
           capture_error || io_error ? "failed" : "captured", (unsigned)geteuid(),
           (unsigned long long)capture_flags, capture_error, io_error, size, finish - start);
    return capture_error || io_error ? 1 : 0;
}
