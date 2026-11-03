#pragma D option quiet
#pragma D option bufsize=1m
#pragma D option switchrate=10hz

uint64_t began;
uint64_t first;
uint64_t target_pid;
uint64_t target_tid;
int baseline;

dtrace:::BEGIN
{
    began = timestamp;
    printf("HVTRACE READY timestamp=%llu uid=%d\n", timestamp, uid);
}

profile:::tick-100ms
/!baseline && timestamp - began >= 1000000000/
{
    baseline = 1;
    printf("HVTRACE BASELINE timestamp=%llu uid=%d\n", timestamp, uid);
}

fbt:com.apple.driver.AppleHV:_ZN13hv_vmx_vcpu_t26THREAD_TRAP_vcpu_run_untilEPS_y:entry
/execname == "hv-control" && uid == $1 &&
    (!target_pid || (pid == target_pid && tid == target_tid)) &&
    (!first || timestamp - first < 3000000000)/
{
    target_pid = target_pid ? target_pid : pid;
    target_tid = target_tid ? target_tid : tid;
    first = first ? first : timestamp;
    printf("HVTRACE run_until.entry timestamp=%llu pid=%d tid=%d uid=%d\n", timestamp, pid, tid, uid);
    stack(8);
}

fbt:com.apple.driver.AppleHV:_ZN13hv_vmx_vcpu_t26THREAD_TRAP_vcpu_run_untilEPS_y:return,
fbt:com.apple.driver.AppleHV:_ZN13hv_vmx_vcpu_t11enter_guestEPjS0_:entry,
fbt:com.apple.driver.AppleHV:_ZN13hv_vmx_vcpu_t11enter_guestEPjS0_:return
/target_pid && pid == target_pid && tid == target_tid && timestamp - first < 3000000000/
{
    printf("HVTRACE %s.%s timestamp=%llu pid=%d tid=%d uid=%d\n", probefunc, probename, timestamp, pid, tid, uid);
    stack(8);
}

profile:::profile-10
/target_pid && pid == target_pid && tid == target_tid && timestamp - first < 3000000000/
{
    printf("HVTRACE SAMPLE timestamp=%llu pid=%d tid=%d uid=%d\n", timestamp, pid, tid, uid);
    stack(8);
}

profile:::tick-100ms
/first && timestamp - first >= 3000000000/
{
    printf("HVTRACE WINDOW_END timestamp=%llu pid=%d tid=%d\n", timestamp, target_pid, target_tid);
    exit(0);
}

profile:::tick-100ms
/timestamp - began >= 20000000000/
{
    printf("HVTRACE DEADLINE timestamp=%llu\n", timestamp);
    exit(2);
}

dtrace:::ERROR
{
    printf("HVTRACE ERROR timestamp=%llu\n", timestamp);
    exit(2);
}
