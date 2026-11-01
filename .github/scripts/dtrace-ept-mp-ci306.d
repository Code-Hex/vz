#pragma D option quiet
#pragma D option bufsize=4m
#pragma D option dynvarsize=1m
#pragma D option switchrate=1hz

/* UUID F6A1B3D4-1EBB-3628-A1C5-89506C51DDEC only: pm_cr3 offset 0x40. */
dtrace:::BEGIN
{
    started = timestamp;
    printf("BEGIN t=%llu wall=%lld\n", timestamp, walltimestamp);
}

fbt:mach_kernel:mp_cpus_call1:entry
{
    self->md++;
    mt[tid, self->md] = timestamp;
    printf("MP_ENTER t=%llu cpu=%d pid=%d tid=%d depth=%d mask=%llx mode=%llu callback=%llx callback_arg0=%llx callback_arg1=%llx caller=%llx\n",
        timestamp, cpu, pid, tid, self->md, (uint64_t)arg0, (uint64_t)arg1, (uint64_t)arg2, (uint64_t)arg3, (uint64_t)arg4, (uint64_t)caller);
}

fbt:mach_kernel:mp_cpus_call1:return
/self->md != 0/
{
    printf("MP_RETURN t=%llu cpu=%d pid=%d tid=%d depth=%d ns=%llu offset=%llu result=%llx\n",
        timestamp, cpu, pid, tid, self->md,
        timestamp - mt[tid, self->md], (uint64_t)arg0, (uint64_t)arg1);
    mt[tid, self->md] = 0;
    self->md--;
}

fbt:mach_kernel:pmap_flush_tlbs:entry
{
    self->pd++;
}

fbt:mach_kernel:pmap_flush_tlbs:entry
/(uint64_t)arg0 != 0 && *(uint64_t *)((uint64_t)arg0 + 0x40) == 0/
{
    pt[tid, self->pd] = timestamp;
    printf("EPT_ENTER t=%llu cpu=%d pid=%d tid=%d depth=%d pmap=%llx start=%llx end=%llx options=%llx context=%llx\n",
        timestamp, cpu, pid, tid, self->pd, (uint64_t)arg0, (uint64_t)arg1, (uint64_t)arg2, (uint64_t)arg3, (uint64_t)arg4);
}

fbt:mach_kernel:pmap_flush_tlbs:return
/self->pd != 0 && pt[tid, self->pd] != 0/
{
    printf("EPT_RETURN t=%llu cpu=%d pid=%d tid=%d depth=%d ns=%llu offset=%llu\n",
        timestamp, cpu, pid, tid, self->pd,
        timestamp - pt[tid, self->pd], (uint64_t)arg0);
    pt[tid, self->pd] = 0;
}

fbt:mach_kernel:pmap_flush_tlbs:return
/self->pd != 0/
{
    self->pd--;
}

profile:::tick-1s
{
    printf("TICK t=%llu wall=%lld cpu=%d elapsed_ns=%llu\n",
        timestamp, walltimestamp, cpu, timestamp - started);
}

profile:::tick-1s
/timestamp - started >= 120000000000/
{
    exit(0);
}

dtrace:::ERROR
{
    printf("ERROR t=%llu cpu=%d epid=%llu action=%llu offset=%llu fault=%llu address=%llx\n",
        timestamp, cpu, (uint64_t)arg1, (uint64_t)arg2, (uint64_t)arg3, (uint64_t)arg4, (uint64_t)arg5);
}

dtrace:::END
{
    printf("END t=%llu wall=%lld\n", timestamp, walltimestamp);
}
