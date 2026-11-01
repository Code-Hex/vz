#pragma D option quiet
#pragma D option bufsize=4m
#pragma D option switchrate=1hz

dtrace:::BEGIN
{
    started = timestamp;
    printf("PROFILE_BEGIN t=%llu wall=%lld\n", timestamp, walltimestamp);
}

profile:::profile-10
{
    printf("CPU_SAMPLE t=%llu cpu=%d pid=%d tid=%d kernel_pc=%llx user_pc=%llx late_ns=%lld\n",
        timestamp, cpu, pid, tid, (uint64_t)arg0, (uint64_t)arg1, (int64_t)arg2);
}

profile:::profile-10
/(uint64_t)arg0 != 0/
{
    stack(8);
}

profile:::tick-1s
/timestamp - started >= 120000000000/
{
    exit(0);
}

dtrace:::ERROR
{
    printf("PROFILE_ERROR t=%llu cpu=%d fault=%llu address=%llx\n", timestamp, cpu, (uint64_t)arg4, (uint64_t)arg5);
}

dtrace:::END
{
    printf("PROFILE_END t=%llu wall=%lld\n", timestamp, walltimestamp);
}
