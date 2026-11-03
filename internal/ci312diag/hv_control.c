// Temporary Intel Hypervisor control. The controller never creates a VM.
#if !defined(__x86_64__)
#error Build this diagnostic for x86_64; do not run Intel VM workloads on ARM.
#endif
#ifndef HV_CONTROL_SOURCE_SHA
#error Define HV_CONTROL_SOURCE_SHA to the SHA-256 of this source when building.
#endif
#include <Hypervisor/hv.h>
#include <Hypervisor/hv_vmx.h>
#include <errno.h>
#include <fcntl.h>
#include <limits.h>
#include <mach-o/dyld.h>
#include <mach/mach_time.h>
#include <poll.h>
#include <signal.h>
#include <spawn.h>
#include <stdbool.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/mman.h>
#include <sys/sysctl.h>
#include <sys/wait.h>
#include <unistd.h>

extern char **environ;
enum { PAGE_BYTES = 4096,
    CODE = 0x100,
    SPIN = 0x200,
    SENTINEL = 0x800,
    COMMAND_FD = 3,
    EVENT_FD = 4,
    MAX_RUNS = 10000,
    RING_SIZE = 512 };
enum event_kind { NOTE,
    BEFORE,
    AFTER,
    WARMUP,
    READY,
    ITERATION_DONE,
    DONE,
    FAILURE,
    ACTIVE_INTERVAL,
    WORKLOAD_INTERVAL };
// Intel SDM Vol. 3C (326019), sections 24.6, 26.3.1, and Appendix C.
// https://www.intel.com/content/dam/www/public/us/en/documents/manuals/64-ia-32-architectures-software-developer-vol-3c-part-3-manual.pdf
// Other exit reasons are deliberately not accepted.
enum { EXIT_INTERRUPT = 1,
    EXIT_HLT = 12,
    EXIT_TIMER = 52 };
struct event {
    uint64_t time_ns, sequence, runs, cap_hash, x, y, z;
    int32_t pid, result;
    uint32_t iteration, kind;
    char role, phase[31], api[96];
};
_Static_assert(sizeof(struct event) < PIPE_BUF, "events must be atomic pipe writes");
static const char *variant;
static char role = 'P';
static const char *phase = "startup";
static unsigned iteration;
static uint64_t sequence, runs, cap_hash;
static mach_timebase_info_data_t timebase;
static uint64_t child_deadline;
static hv_vcpuid_t vcpu;
static bool vm_created, memory_mapped, cpu_created;
static uint8_t *memory;
static int cleanup_error;

static uint64_t now_ns(void)
{
    return (uint64_t)((__uint128_t)mach_absolute_time() * timebase.numer / timebase.denom);
}

struct output_line {
    size_t size, offset;
    char bytes[768];
};
static struct output_line ring[RING_SIZE];
static unsigned ring_head, ring_count;
static uint64_t dropped;
static bool output_error;

static void flush_output(void)
{
    while (ring_count) {
        struct output_line *line = &ring[ring_head];
        ssize_t n = write(STDOUT_FILENO, line->bytes + line->offset, line->size - line->offset);
        if (n < 0 && errno == EINTR)
            continue;
        if (n < 0 && (errno == EAGAIN || errno == EWOULDBLOCK))
            return;
        if (n <= 0) {
            output_error = true;
            return;
        }
        line->offset += (size_t)n;
        if (line->offset != line->size)
            return;
        ring_head = (ring_head + 1) % RING_SIZE;
        --ring_count;
    }
}

static void output_event(const struct event *e)
{
    flush_output();
    if (ring_count == RING_SIZE) {
        ++dropped;
        return;
    }
    struct output_line *line = &ring[(ring_head + ring_count) % RING_SIZE];
    int n = snprintf(line->bytes, sizeof(line->bytes),
        "HVCONTROL source=%s case=%s role=%c pid=%d mono_ns=%llu seq=%llu phase=%s kind=%u iter=%u runs=%llu cap_hash=%016llx api=%s rc=%d rc_hex=%08x x=%llu y=%llu z=%llu dropped=%llu\n",
        HV_CONTROL_SOURCE_SHA, variant, e->role, e->pid,
        (unsigned long long)e->time_ns, (unsigned long long)e->sequence,
        e->phase, e->kind, e->iteration, (unsigned long long)e->runs,
        (unsigned long long)e->cap_hash, e->api, e->result, (unsigned)e->result,
        (unsigned long long)e->x, (unsigned long long)e->y, (unsigned long long)e->z,
        (unsigned long long)dropped);
    if (n < 0 || (size_t)n >= sizeof(line->bytes)) {
        output_error = true;
        return;
    }
    line->size = (size_t)n;
    line->offset = 0;
    ++ring_count;
    flush_output();
}

static void record(unsigned kind, const char *api, int result, uint64_t x, uint64_t y, uint64_t z)
{
    struct event e = { .time_ns = now_ns(), .sequence = ++sequence, .runs = runs, .cap_hash = cap_hash, .x = x, .y = y, .z = z, .pid = getpid(), .result = result, .iteration = iteration, .kind = kind, .role = role };
    if (strlcpy(e.phase, phase, sizeof(e.phase)) >= sizeof(e.phase)
        || strlcpy(e.api, api, sizeof(e.api)) >= sizeof(e.api))
        _exit(74);
    if (role == 'P') {
        output_event(&e);
        return;
    }
    // Initialization can burst beyond pipe capacity. Bound transport waiting,
    // while the controller drains independently and keeps its heartbeat alive.
    uint64_t until = now_ns() + 250000000;
    for (;;) {
        ssize_t n = write(EVENT_FD, &e, sizeof(e));
        if (n == sizeof(e))
            return;
        if (n < 0 && errno == EINTR && now_ns() < until)
            continue;
        if (n < 0 && (errno == EAGAIN || errno == EWOULDBLOCK) && now_ns() < until) {
            struct pollfd fd = { .fd = EVENT_FD, .events = POLLOUT };
            poll(&fd, 1, 5);
            continue;
        }
        // Lost protocol evidence invalidates the run, including implicit teardown.
        _exit(74);
    }
}

static int failure(const char *why, int result)
{
    record(FAILURE, why, result, 0, 0, 0);
    return result;
}

static int checked(const char *api, hv_return_t result)
{
    record(AFTER, api, result, 0, 0, 0);
    if (result == HV_SUCCESS)
        return 0;
    return failure("NOT_ESTABLISHED_API", result == HV_UNSUPPORTED || result == HV_NO_DEVICE ? 77 : result == HV_DENIED ? 78
                                                                                                                        : 70);
}
#define CALL(name, expr) (record(BEFORE, name, 0, 0, 0, 0), checked(name, (expr)))
#define TRY(expr)             \
    do {                      \
        int result_ = (expr); \
        if (result_)          \
            return result_;   \
    } while (0)

static void hash_value(uint64_t value)
{
    for (unsigned i = 0; i < 8; ++i) {
        cap_hash ^= (value >> (i * 8)) & 255;
        cap_hash *= UINT64_C(1099511628211);
    }
}

static int write_field(uint32_t field, uint64_t value)
{
    record(NOTE, "VMCS_WRITE_VALUE", 0, field, value, 0);
    TRY(CALL("hv_vmx_vcpu_write_vmcs", hv_vmx_vcpu_write_vmcs(vcpu, field, value)));
    uint64_t actual = 0;
    TRY(CALL("hv_vmx_vcpu_read_vmcs", hv_vmx_vcpu_read_vmcs(vcpu, field, &actual)));
    if (actual != value)
        return failure("VMCS_READBACK_MISMATCH", 70);
    return 0;
}

static int read_field(uint32_t field, uint64_t *value)
{
    return CALL("hv_vmx_vcpu_read_vmcs", hv_vmx_vcpu_read_vmcs(vcpu, field, value));
}

static int control(uint32_t field, uint64_t desired, uint64_t forbidden, uint64_t *applied)
{
    uint64_t must_one = 0, may_one = 0;
    TRY(CALL("hv_vmx_vcpu_get_cap_write_vmcs", hv_vmx_vcpu_get_cap_write_vmcs(vcpu, field, &must_one, &may_one)));
    record(NOTE, "CONTROL_CAP", 0, field, must_one, may_one);
    hash_value(field);
    hash_value(must_one);
    hash_value(may_one);
    // Apple's allowed_0 is a must-be-one mask, not its complement.
    if ((must_one & ~may_one) || (desired & ~may_one) || (must_one & forbidden))
        return failure("NOT_ESTABLISHED_CONTROL", 77);
    TRY(write_field(field, must_one | desired));
    if (applied)
        *applied = must_one | desired;
    return 0;
}

static int capability(hv_vmx_capability_t field, uint64_t *value)
{
    TRY(CALL("hv_vmx_read_capability", hv_vmx_read_capability(field, value)));
    record(NOTE, "HOST_CAP", 0, field, *value, 0);
    hash_value(field);
    hash_value(*value);
    return 0;
}

static int initialize_cpu(void)
{
    phase = "initialize";
    cap_hash = UINT64_C(14695981039346656037);
    uint64_t cr0_fixed0, cr0_fixed1, cr4_fixed0, cr4_fixed1, entry_controls;
    TRY(capability(HV_VMX_CAP_CR0_FIXED0, &cr0_fixed0));
    TRY(capability(HV_VMX_CAP_CR0_FIXED1, &cr0_fixed1));
    TRY(capability(HV_VMX_CAP_CR4_FIXED0, &cr4_fixed0));
    TRY(capability(HV_VMX_CAP_CR4_FIXED1, &cr4_fixed1));
    TRY(control(VMCS_CTRL_PIN_BASED, 0, 0, NULL));
    TRY(control(VMCS_CTRL_CPU_BASED, (1ULL << 31) | (1ULL << 7), (1ULL << 2) | (1ULL << 22) | (1ULL << 27), NULL));
    TRY(control(VMCS_CTRL_CPU_BASED2, (1ULL << 1) | (1ULL << 7), (1ULL << 14), NULL));
    TRY(control(VMCS_CTRL_VMENTRY_CONTROLS, 0, (1ULL << 9) | (1ULL << 10) | (1ULL << 11), &entry_controls));
    // Intel SDM 26.3.1: unrestricted guest relaxes only CR0.PE/PG fixed-one checks.
    uint64_t cr0 = (cr0_fixed0 | (1ULL << 4)) & ~((1ULL << 31) | 1ULL);
    uint64_t cr4 = cr4_fixed0;
    if ((cr0 & ~cr0_fixed1) || (cr4 & ~cr4_fixed1)
        || (cr0 & ((1ULL << 29) | (1ULL << 30))) || (cr4 & (1ULL << 17)))
        return failure("NOT_ESTABLISHED_CR_FIXED", 77);
    TRY(write_field(VMCS_GUEST_CR0, cr0));
    TRY(write_field(VMCS_GUEST_CR3, 0));
    TRY(write_field(VMCS_GUEST_CR4, cr4));
    const struct {
        uint32_t selector, base, limit, access;
        uint64_t rights;
    } segments[] = {
        { VMCS_GUEST_CS, VMCS_GUEST_CS_BASE, VMCS_GUEST_CS_LIMIT, VMCS_GUEST_CS_AR, 0x9b },
        { VMCS_GUEST_DS, VMCS_GUEST_DS_BASE, VMCS_GUEST_DS_LIMIT, VMCS_GUEST_DS_AR, 0x93 },
        { VMCS_GUEST_ES, VMCS_GUEST_ES_BASE, VMCS_GUEST_ES_LIMIT, VMCS_GUEST_ES_AR, 0x93 },
        { VMCS_GUEST_SS, VMCS_GUEST_SS_BASE, VMCS_GUEST_SS_LIMIT, VMCS_GUEST_SS_AR, 0x93 },
        { VMCS_GUEST_FS, VMCS_GUEST_FS_BASE, VMCS_GUEST_FS_LIMIT, VMCS_GUEST_FS_AR, 0x93 },
        { VMCS_GUEST_GS, VMCS_GUEST_GS_BASE, VMCS_GUEST_GS_LIMIT, VMCS_GUEST_GS_AR, 0x93 },
        { VMCS_GUEST_TR, VMCS_GUEST_TR_BASE, VMCS_GUEST_TR_LIMIT, VMCS_GUEST_TR_AR, 0x8b },
        { VMCS_GUEST_LDTR, VMCS_GUEST_LDTR_BASE, VMCS_GUEST_LDTR_LIMIT, VMCS_GUEST_LDTR_AR, 0x10000 },
    };
    for (unsigned i = 0; i < sizeof(segments) / sizeof(segments[0]); ++i) {
        TRY(write_field(segments[i].selector, 0));
        TRY(write_field(segments[i].base, 0));
        TRY(write_field(segments[i].limit, 0xffff));
        TRY(write_field(segments[i].access, segments[i].rights));
    }
    const struct {
        uint32_t field;
        uint64_t value;
    } fields[] = {
        { VMCS_GUEST_GDTR_BASE, 0 },
        { VMCS_GUEST_GDTR_LIMIT, 0xffff },
        { VMCS_GUEST_IDTR_BASE, 0 },
        { VMCS_GUEST_IDTR_LIMIT, 0x3ff },
        { VMCS_GUEST_RIP, CODE },
        { VMCS_GUEST_RSP, 0xff0 },
        { VMCS_GUEST_RFLAGS, 2 },
        { VMCS_GUEST_DR7, 0x400 },
        { VMCS_GUEST_IA32_EFER, 0 },
        { VMCS_GUEST_INTERRUPTIBILITY, 0 },
        { VMCS_GUEST_ACTIVITY_STATE, 0 },
        { VMCS_GUEST_DEBUG_EXC, 0 },
        { VMCS_GUEST_IA32_SYSENTER_CS, 0 },
        { VMCS_GUEST_SYSENTER_ESP, 0 },
        { VMCS_GUEST_SYSENTER_EIP, 0 },
        { VMCS_CTRL_VMENTRY_IRQ_INFO, 0 },
        { VMCS_CTRL_VMENTRY_EXC_ERROR, 0 },
        { VMCS_CTRL_VMENTRY_INSTR_LEN, 0 },
        { VMCS_CTRL_EXC_BITMAP, UINT32_MAX },
        { VMCS_CTRL_PF_ERROR_MASK, 0 },
    };
    for (unsigned i = 0; i < sizeof(fields) / sizeof(fields[0]); ++i)
        TRY(write_field(fields[i].field, fields[i].value));
    // Fields outside Apple's public VMCS access table remain framework-owned.
    // Successful sentinel + HLT execution is the VM-entry validity gate.
    // Intel IA32_DEBUGCTL (0x1d9) is relevant when entry controls load debug state.
    if (entry_controls & (1ULL << 2)) {
        TRY(CALL("hv_vcpu_write_msr_DEBUGCTL", hv_vcpu_write_msr(vcpu, 0x1d9, 0)));
        uint64_t debugctl = UINT64_MAX;
        TRY(CALL("hv_vcpu_read_msr_DEBUGCTL", hv_vcpu_read_msr(vcpu, 0x1d9, &debugctl)));
        if (debugctl != 0)
            return failure("DEBUGCTL_READBACK_MISMATCH", 70);
    }
    record(NOTE, "CAP_COMPLETE", 0, cr0, cr4, entry_controls);
    return 0;
}

struct interval {
    uint64_t start, end, execution_ns;
};

static bool qualifying_overlap(const struct interval *active, const struct interval *workload, uint64_t ready)
{
    return ready && active->start >= ready && active->execution_ns > 0
        && active->start < active->end && workload->start < workload->end
        && active->start < workload->end && workload->start < active->end;
}

static int run_quantum(uint64_t *reason, uint64_t *rip, struct interval *interval)
{
    if (now_ns() >= child_deadline || runs >= MAX_RUNS)
        return failure("CHILD_DEADLINE_OR_RUN_LIMIT", 124);
    uint64_t ticks = (uint64_t)((__uint128_t)20000000 * timebase.denom / timebase.numer);
    uint64_t deadline = mach_absolute_time() + (ticks ? ticks : 1);
    ++runs;
    record(NOTE, "RUN_DEADLINE_ABSOLUTE", 0, deadline, 20000000, 0);
    record(BEFORE, "hv_vcpu_run_until", 0, 0, 0, 0);
    uint64_t entered = mach_absolute_time();
    hv_return_t result = hv_vcpu_run_until(vcpu, deadline);
    uint64_t returned = mach_absolute_time();
    TRY(checked("hv_vcpu_run_until", result));
    if (interval) {
        interval->start = entered;
        interval->end = returned;
    }
    TRY(read_field(VMCS_RO_EXIT_REASON, reason));
    TRY(read_field(VMCS_GUEST_RIP, rip));
    record(NOTE, "VMEXIT", 0, *reason, *rip, 0);
    if (*reason & (1ULL << 31))
        return failure("VM_ENTRY_FAILURE", 70);
    return 0;
}

static int create_and_warmup(void)
{
    phase = "create";
    TRY(CALL("hv_vm_create", hv_vm_create(HV_VM_DEFAULT)));
    vm_created = true;
    memory = mmap(NULL, PAGE_BYTES, PROT_READ | PROT_WRITE, MAP_PRIVATE | MAP_ANON, -1, 0);
    if (memory == MAP_FAILED) {
        memory = NULL;
        return failure("mmap", 71);
    }
    uint8_t tag = role == 'A' ? 0xa1 : (uint8_t)iteration;
    memset(memory, 0, PAGE_BYTES);
    const uint8_t code[] = { 0xc6, 0x06, 0x00, 0x08, tag, 0xf4 };
    memcpy(memory + CODE, code, sizeof(code));
    memory[SPIN] = 0xeb;
    memory[SPIN + 1] = 0xfe;
    TRY(CALL("hv_vm_map", hv_vm_map(memory, 0, PAGE_BYTES, HV_MEMORY_READ | HV_MEMORY_WRITE | HV_MEMORY_EXEC)));
    memory_mapped = true;
    TRY(CALL("hv_vcpu_create", hv_vcpu_create(&vcpu, HV_VCPU_DEFAULT)));
    cpu_created = true;
    TRY(initialize_cpu());
    phase = "warmup";
    if (*(volatile uint8_t *)(memory + SENTINEL) != 0)
        return failure("SENTINEL_NOT_ZERO", 70);
    uint64_t until = now_ns() + 2000000000ULL;
    for (unsigned attempt = 0; attempt < 100 && now_ns() < until; ++attempt) {
        uint64_t reason, rip, length;
        TRY(run_quantum(&reason, &rip, NULL));
        if ((reason & 0xffff) == EXIT_HLT) {
            TRY(read_field(VMCS_RO_VMEXIT_INSTR_LEN, &length));
            uint8_t actual = *(volatile uint8_t *)(memory + SENTINEL);
            record(NOTE, "WARMUP_EVIDENCE", 0, actual, rip, length);
            if (actual != tag || rip != CODE + 5 || length != 1)
                return failure("WARMUP_MISMATCH", 70);
            record(WARMUP, "SENTINEL_HLT_VERIFIED", 0, tag, rip, reason);
            return 0;
        }
        if (((reason & 0xffff) != EXIT_INTERRUPT && (reason & 0xffff) != EXIT_TIMER)
            || (rip != CODE && rip != CODE + 5))
            return failure("UNEXPECTED_WARMUP_EXIT", 70);
    }
    return failure("WARMUP_NOT_COMPLETED", 124);
}

static int cleanup(void)
{
    phase = "cleanup";
    if (cleanup_error)
        return cleanup_error;
    // Stop on cleanup failure, retaining mappings until implicit process teardown.
    if (cpu_created) {
        cleanup_error = CALL("hv_vcpu_destroy", hv_vcpu_destroy(vcpu));
        if (cleanup_error)
            return cleanup_error;
        cpu_created = false;
    }
    if (memory_mapped) {
        cleanup_error = CALL("hv_vm_unmap", hv_vm_unmap(0, PAGE_BYTES));
        if (cleanup_error)
            return cleanup_error;
        memory_mapped = false;
    }
    if (vm_created) {
        cleanup_error = CALL("hv_vm_destroy", hv_vm_destroy());
        if (cleanup_error)
            return cleanup_error;
        vm_created = false;
    }
    if (memory && munmap(memory, PAGE_BYTES) != 0) {
        cleanup_error = failure("munmap", 71);
        return cleanup_error;
    }
    memory = NULL;
    return 0;
}

static int command(int wait_ms, char *value)
{
    *value = 0;
    if (now_ns() >= child_deadline)
        return failure("CHILD_SOFT_DEADLINE", 124);
    struct pollfd fd = { .fd = COMMAND_FD, .events = POLLIN };
    int ready = poll(&fd, 1, wait_ms);
    if (ready < 0 && errno == EINTR)
        return 0;
    if (ready < 0)
        return failure("command_poll", 71);
    if (!ready)
        return 0;
    if (read(COMMAND_FD, value, 1) != 1)
        return failure("COMMAND_CLOSED", 71);
    if (*value != 'G' && *value != 'S')
        return failure("COMMAND_INVALID", 65);
    return 0;
}

static int spin_once(uint64_t *execution)
{
    uint64_t reason, rip, previous = *execution;
    struct interval interval = { 0 };
    TRY(run_quantum(&reason, &rip, &interval));
    TRY(CALL("hv_vcpu_get_exec_time", hv_vcpu_get_exec_time(vcpu, execution)));
    record(NOTE, "ACTIVE_EVIDENCE", 0, previous, *execution, reason);
    if (((reason & 0xffff) != EXIT_INTERRUPT && (reason & 0xffff) != EXIT_TIMER)
        || rip != SPIN || *execution <= previous)
        return failure("ACTIVE_NOT_ESTABLISHED", 70);
    record(ACTIVE_INTERVAL, "ACTIVE_RUN_INTERVAL", 0, interval.start, interval.end, *execution - previous);
    return 0;
}

static int hold_a(void)
{
    iteration = 1;
    TRY(create_and_warmup());
    uint64_t execution = 0;
    TRY(CALL("hv_vcpu_get_exec_time", hv_vcpu_get_exec_time(vcpu, &execution)));
    bool active = strcmp(variant, "active") == 0;
    phase = active ? "active" : "park";
    if (active) {
        TRY(write_field(VMCS_GUEST_RIP, SPIN));
        TRY(spin_once(&execution));
        TRY(spin_once(&execution));
    }
    uint64_t parked_runs = runs, parked_execution = execution;
    record(READY, active ? "ACTIVE_READY" : "PARK_READY", 0, execution, runs, mach_absolute_time());
    for (;;) {
        char value;
        TRY(command(active ? 0 : 250, &value));
        if (value == 'S')
            break;
        if (value)
            return failure("UNEXPECTED_A_COMMAND", 65);
        if (active)
            TRY(spin_once(&execution));
        else
            record(NOTE, "PARK_HEARTBEAT", 0, parked_execution, parked_runs, runs);
    }
    record(NOTE, "STOP_ACK_RUN_RETURNED", 0, runs, execution, 0);
    if (!active) {
        TRY(CALL("hv_vcpu_get_exec_time", hv_vcpu_get_exec_time(vcpu, &execution)));
        if (runs != parked_runs || execution != parked_execution)
            return failure("PARK_INVARIANT_FAILED", 70);
        record(NOTE, "PARK_INVARIANT_VERIFIED", 0, runs, execution, 0);
    }
    return 0;
}

static int churn_b(unsigned count)
{
    phase = "wait_start";
    char value = 0;
    while (!value)
        TRY(command(250, &value));
    if (value != 'G')
        return failure("B_CANCELLED_BEFORE_START", 75);
    for (iteration = 1; iteration <= count; ++iteration) {
        uint64_t entered = mach_absolute_time();
        TRY(create_and_warmup());
        TRY(cleanup());
        uint64_t returned = mach_absolute_time();
        record(WORKLOAD_INTERVAL, "B_LIFECYCLE_INTERVAL", 0, entered, returned, 0);
        record(ITERATION_DONE, "WARMUP_AND_TEARDOWN_COMPLETE", 0, 0, 0, 0);
        if (iteration == count)
            break;
        phase = "interval";
        uint64_t next = now_ns() + 500000000;
        while (now_ns() < next) {
            TRY(command(10, &value));
            if (value)
                return failure("B_CANCELLED", 75);
        }
    }
    return 0;
}

static int child(unsigned count)
{
    child_deadline = now_ns() + 120000000000ULL;
    int supported = 0;
    size_t size = sizeof(supported);
    if (sysconf(_SC_PAGESIZE) != PAGE_BYTES
        || sysctlbyname("kern.hv_support", &supported, &size, NULL, 0) || supported != 1)
        return failure("NOT_ESTABLISHED_PLATFORM", 77);
    record(NOTE, "GUEST_LAYOUT", 0, CODE, SPIN, SENTINEL);
    int result = role == 'A' ? hold_a() : churn_b(count);
    int cleanup_result = cleanup();
    if (cleanup_result) {
        record(FAILURE, "IMPLICIT_PROCESS_TEARDOWN", cleanup_result, cpu_created, memory_mapped, vm_created);
        return cleanup_result;
    }
    if (result)
        return result;
    phase = "complete";
    record(DONE, "CHILD_COMPLETE", 0, 0, 0, 0);
    return 0;
}

struct child_state {
    pid_t pid;
    int command, event, status;
    bool exited, eof, ready, done;
    unsigned warmups, completed;
    uint64_t runs, last_sequence;
    char phase[31];
    size_t used;
    struct event partial;
};

static int nonblock(int fd)
{
    int flags = fcntl(fd, F_GETFL);
    return flags < 0 || fcntl(fd, F_SETFL, flags | O_NONBLOCK) < 0 ? -1 : 0;
}

static int spawn_child(struct child_state *c, const char *path, char child_role, const char *count)
{
    int command_pipe[2], event_pipe[2];
    if (pipe(command_pipe))
        return 71;
    if (pipe(event_pipe)) {
        close(command_pipe[0]);
        close(command_pipe[1]);
        return 71;
    }
    int fds[] = { command_pipe[0], command_pipe[1], event_pipe[0], event_pipe[1] };
    for (unsigned i = 0; i < 4; ++i) {
        if (fcntl(fds[i], F_SETFD, FD_CLOEXEC) < 0 || nonblock(fds[i])) {
            for (unsigned j = 0; j < 4; ++j)
                close(fds[j]);
            return 71;
        }
    }
    posix_spawn_file_actions_t actions;
    int result = posix_spawn_file_actions_init(&actions);
    if (!result) {
        // Source descriptors are >=10, so dup2 cannot overwrite another source.
        int in = fcntl(command_pipe[0], F_DUPFD_CLOEXEC, 10);
        int out = fcntl(event_pipe[1], F_DUPFD_CLOEXEC, 10);
        if (in < 0 || out < 0)
            result = errno;
        if (!result)
            result = posix_spawn_file_actions_adddup2(&actions, in, COMMAND_FD);
        if (!result)
            result = posix_spawn_file_actions_adddup2(&actions, out, EVENT_FD);
        if (!result)
            result = posix_spawn_file_actions_addclose(&actions, in);
        if (!result)
            result = posix_spawn_file_actions_addclose(&actions, out);
        char role_arg[] = { child_role, 0 };
        char *args[] = { (char *)path, "--child", role_arg, (char *)variant, (char *)count, NULL };
        if (!result)
            result = posix_spawn(&c->pid, path, &actions, NULL, args, environ);
        if (in >= 0)
            close(in);
        if (out >= 0)
            close(out);
        posix_spawn_file_actions_destroy(&actions);
    }
    close(command_pipe[0]);
    close(event_pipe[1]);
    if (result) {
        close(command_pipe[1]);
        close(event_pipe[0]);
        return failure("posix_spawn", 71);
    }
    c->command = command_pipe[1];
    c->event = event_pipe[0];
    record(NOTE, "SPAWNED", 0, c->pid, child_role, 0);
    return 0;
}

static int send_command(struct child_state *c, char value)
{
    if (!c->pid || c->exited)
        return 0;
    record(BEFORE, value == 'G' ? "SEND_GO" : "SEND_STOP", 0, c->pid, 0, 0);
    ssize_t n;
    do {
        n = write(c->command, &value, 1);
    } while (n < 0 && errno == EINTR);
    if (n != 1)
        return failure("COMMAND_WRITE_FAILED", 71);
    return 0;
}

static struct interval active_intervals[MAX_RUNS], workload_intervals[100];
static unsigned active_count, workload_count;
static uint64_t a_ready;

static int collect_interval(const struct event *e)
{
    if (e->kind == ACTIVE_INTERVAL) {
        if (e->role != 'A' || active_count == MAX_RUNS || e->x >= e->y || !e->z)
            return failure("INVALID_ACTIVE_INTERVAL", 74);
        active_intervals[active_count++] = (struct interval) { e->x, e->y, e->z };
    } else if (e->kind == WORKLOAD_INTERVAL) {
        if (e->role != 'B' || workload_count == 100 || e->x >= e->y
            || e->iteration != workload_count + 1)
            return failure("INVALID_WORKLOAD_INTERVAL", 74);
        workload_intervals[workload_count++] = (struct interval) { e->x, e->y, 0 };
    } else if (e->kind == READY && e->role == 'A')
        a_ready = e->z;
    return 0;
}

static bool verify_active_overlap(void)
{
    // Compare child clock values only after both event pipes have reached EOF.
    // This proves call-interval overlap, not simultaneous physical CPU execution.
    for (unsigned i = 0; i < active_count; ++i)
        for (unsigned j = 0; j < workload_count; ++j)
            if (qualifying_overlap(&active_intervals[i], &workload_intervals[j], a_ready)) {
                record(NOTE, "ACTIVE_B_OVERLAP_VERIFIED", 0, i + 1, j + 1, active_intervals[i].execution_ns);
                return true;
            }
    return false;
}

static int drain(struct child_state *c, char expected_role)
{
    if (!c->pid || c->eof)
        return 0;
    // A finite drain batch keeps the parent's heartbeat schedulable.
    for (unsigned batch = 0; batch < 128; ++batch) {
        ssize_t n = read(c->event, (char *)&c->partial + c->used, sizeof(c->partial) - c->used);
        if (n < 0 && errno == EINTR)
            continue;
        if (n < 0 && (errno == EAGAIN || errno == EWOULDBLOCK))
            return 0;
        if (n < 0)
            return failure("EVENT_READ_FAILED", 71);
        if (!n) {
            c->eof = true;
            return c->used ? failure("TRUNCATED_EVENT", 74) : 0;
        }
        c->used += (size_t)n;
        if (c->used != sizeof(c->partial))
            continue;
        c->used = 0;
        struct event *e = &c->partial;
        if (e->pid != c->pid || e->role != expected_role || e->sequence != c->last_sequence + 1
            || !memchr(e->phase, 0, sizeof(e->phase)) || !memchr(e->api, 0, sizeof(e->api)))
            return failure("INVALID_EVENT", 74);
        c->last_sequence = e->sequence;
        c->runs = e->runs;
        strlcpy(c->phase, e->phase, sizeof(c->phase));
        output_event(e);
        TRY(collect_interval(e));
        if (e->kind == WARMUP)
            ++c->warmups;
        if (e->kind == READY)
            c->ready = true;
        if (e->kind == ITERATION_DONE)
            ++c->completed;
        if (e->kind == DONE)
            c->done = true;
        if (e->kind == FAILURE)
            return e->result ? e->result : 70;
    }
    return 0;
}

static void reap(struct child_state *c)
{
    if (!c->pid || c->exited)
        return;
    pid_t result = waitpid(c->pid, &c->status, WNOHANG);
    if (result == c->pid) {
        c->exited = true;
        record(NOTE, "CHILD_EXIT", 0, c->pid, c->status, c->done);
    }
}

static void signal_child(struct child_state *c, int signal_number)
{
    // An unreaped child owns its PID, preventing PID reuse before kill.
    if (c->pid && !c->exited) {
        int result = kill(c->pid, signal_number);
        record(NOTE, "SIGNAL_IMPLICIT_TEARDOWN", result ? errno : 0, c->pid, signal_number, 0);
    }
}

static void metadata(const char *key)
{
    char value[80] = { 0 };
    size_t size = sizeof(value);
    if (sysctlbyname(key, value, &size, NULL, 0)) {
        record(NOTE, key, errno, 0, 0, 0);
        return;
    }
    for (unsigned i = 0; value[i] && i < sizeof(value) - 1; ++i)
        if (!(value[i] >= '0' && value[i] <= '9') && !(value[i] >= 'a' && value[i] <= 'z')
            && !(value[i] >= 'A' && value[i] <= 'Z') && value[i] != '-' && value[i] != '.')
            value[i] = '_';
    record(NOTE, value, 0, 0, 0, 0);
}

static void image_metadata(const char *key)
{
    const char *value = getenv(key);
    char safe[80];
    if (!value || !*value || strlen(value) >= sizeof(safe)) {
        record(NOTE, key, 0, 0, 0, 0);
        record(NOTE, "NOT_PROVIDED", 0, 0, 0, 0);
        return;
    }
    strlcpy(safe, value, sizeof(safe));
    for (unsigned i = 0; safe[i]; ++i)
        if (!(safe[i] >= '0' && safe[i] <= '9') && !(safe[i] >= 'a' && safe[i] <= 'z')
            && !(safe[i] >= 'A' && safe[i] <= 'Z') && safe[i] != '-' && safe[i] != '.')
            safe[i] = '_';
    record(NOTE, key, 0, 0, 0, 0);
    record(NOTE, safe, 0, 0, 0, 0);
}

static int controller(const char *count_arg, unsigned count)
{
    if (nonblock(STDOUT_FILENO))
        return 71;
    phase = "metadata";
    record(NOTE, "OS_BUILD", 0, 0, 0, 0);
    metadata("kern.osversion");
    record(NOTE, "KERNEL_UUID", 0, 0, 0, 0);
    metadata("kern.uuid");
    record(NOTE, "LAYOUT_4096_CODE_SPIN_SENTINEL", 0, CODE, SPIN, SENTINEL);
    image_metadata("ImageOS");
    image_metadata("ImageVersion");
    phase = "baseline";
    uint64_t start = now_ns(), heartbeat = start;
    while (now_ns() - start < 5000000000ULL) {
        if (now_ns() >= heartbeat) {
            record(NOTE, "PARENT_HEARTBEAT", 0, 0, 0, 0);
            heartbeat += 250000000;
        }
        flush_output();
        poll(NULL, 0, 10);
    }
    char path[PATH_MAX];
    uint32_t path_size = sizeof(path);
    if (_NSGetExecutablePath(path, &path_size))
        return failure("EXECUTABLE_PATH", 71);
    struct child_state a = { 0 }, b = { 0 };
    phase = "spawn";
    int result = spawn_child(&a, path, 'A', count_arg);
    if (!result)
        result = spawn_child(&b, path, 'B', count_arg);
    bool started = false, stopping = false;
    unsigned escalation = 0;
    uint64_t stop_time = 0;
    phase = "observe";
    while ((a.pid && (!a.exited || !a.eof)) || (b.pid && (!b.exited || !b.eof))) {
        int r = drain(&a, 'A');
        if (!r)
            r = drain(&b, 'B');
        if (!result && r)
            result = r;
        reap(&a);
        reap(&b);
        if (!result && ((a.exited && (!WIFEXITED(a.status) || WEXITSTATUS(a.status))) || (b.exited && (!WIFEXITED(b.status) || WEXITSTATUS(b.status)))))
            result = 70;
        if (!result && (output_error || dropped))
            result = 74;
        if (!result && now_ns() - start >= 150000000000ULL)
            result = failure("PARENT_HARD_DEADLINE", 124);
        if (!result && !started && a.ready) {
            if (a.warmups != 1)
                result = failure("READY_WITHOUT_WARMUP", 70);
            else {
                result = send_command(&b, 'G');
                started = !result;
            }
        }
        if (!stopping && (result || b.done || b.exited || a.exited)) {
            stopping = true;
            stop_time = now_ns();
            int stop_a = send_command(&a, 'S');
            int stop_b = b.done ? 0 : send_command(&b, 'S');
            if (!result && (stop_a || stop_b))
                result = 71;
        }
        if (stopping && now_ns() - stop_time >= (escalation ? 3000000000ULL : 2000000000ULL) && escalation < 2) {
            signal_child(&a, escalation ? SIGKILL : SIGTERM);
            signal_child(&b, escalation ? SIGKILL : SIGTERM);
            ++escalation;
            if (!result)
                result = 124;
        }
        if (stopping && now_ns() - stop_time >= 5000000000ULL) {
            if (!result)
                result = 124;
            break;
        }
        if (now_ns() >= heartbeat) {
            phase = "observe";
            record(NOTE, "PARENT_HEARTBEAT_A", 0, a.pid, a.runs, a.exited);
            record(NOTE, a.phase[0] ? a.phase : "not_started", 0, a.warmups, a.completed, a.done);
            record(NOTE, "PARENT_HEARTBEAT_B", 0, b.pid, b.runs, b.exited);
            record(NOTE, b.phase[0] ? b.phase : "not_started", 0, b.warmups, b.completed, b.done);
            heartbeat = now_ns() + 250000000;
        }
        flush_output();
        struct pollfd fds[] = { { .fd = !a.pid || a.eof ? -1 : a.event, .events = POLLIN },
            { .fd = !b.pid || b.eof ? -1 : b.event, .events = POLLIN } };
        poll(fds, 2, 10);
    }
    if (!result && (!started || !a.ready || !a.done || !b.done || a.warmups != 1 || b.warmups != count || b.completed != count || !a.exited || !b.exited))
        result = 70;
    if (!result && (!a.eof || !b.eof || workload_count != count))
        result = failure("INCOMPLETE_INTERVAL_EVIDENCE", 74);
    if (!result && strcmp(variant, "active") == 0 && !verify_active_overlap())
        result = failure("ACTIVE_WITH_B_NOT_ESTABLISHED", 70);
    if (a.pid) {
        close(a.command);
        close(a.event);
    }
    if (b.pid) {
        close(b.command);
        close(b.event);
    }
    phase = "final";
    // Finish queued evidence before recording success; loss always invalidates it.
    uint64_t flush_deadline = now_ns() + 1000000000;
    while (ring_count && now_ns() < flush_deadline) {
        flush_output();
        poll(NULL, 0, 10);
    }
    if (ring_count || dropped || output_error)
        result = 74;
    record(result ? FAILURE : DONE, result ? "INCONCLUSIVE" : "PASS_WARMUP_AND_LIFECYCLE", result,
        a.warmups, b.warmups, b.completed);
    flush_deadline = now_ns() + 1000000000;
    while (ring_count && now_ns() < flush_deadline) {
        flush_output();
        poll(NULL, 0, 10);
    }
    return ring_count || dropped || output_error ? 74 : result;
}

int main(int argc, char **argv)
{
    if (mach_timebase_info(&timebase) != KERN_SUCCESS || !timebase.numer || !timebase.denom)
        return 71;
    signal(SIGPIPE, SIG_IGN);
    bool is_child = argc == 5 && strcmp(argv[1], "--child") == 0;
    if ((!is_child && argc != 3) || (is_child && strcmp(argv[2], "A") && strcmp(argv[2], "B"))) {
        fputs("usage: hv-control active|park 1..100\n", stderr);
        return 64;
    }
    variant = argv[is_child ? 3 : 1];
    const char *count_arg = argv[is_child ? 4 : 2];
    char *end;
    errno = 0;
    unsigned long count = strtoul(count_arg, &end, 10);
    if ((strcmp(variant, "active") && strcmp(variant, "park")) || errno || !*count_arg || *end || count < 1 || count > 100)
        return 64;
    if (is_child) {
        role = argv[2][0];
        return child((unsigned)count);
    }
    return controller(count_arg, (unsigned)count);
}
