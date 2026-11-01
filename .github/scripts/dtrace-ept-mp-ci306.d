#pragma D option quiet
#pragma D option bufsize=4m
#pragma D option dynvarsize=1m
#pragma D option switchrate=1hz

/* UUID F6A1B3D4-1EBB-3628-A1C5-89506C51DDEC, x86_64 only. */
self uint64_t epdepth;
self uint64_t epstart;
self uint64_t mpdepth;
self uint64_t mpstart;

dtrace:::BEGIN
{
    started = timestamp;
    printf("BEGIN t=%llu wall=%lld version=ept-only-1\n", timestamp, walltimestamp);
}

/* This clause must precede the outer-entry clause. */
fbt:mach_kernel:pmap_flush_tlbs:entry
/self->epdepth != 0/
{
    self->epdepth++;
    printf("EPT_NEST_ENTER t=%llu cpu=%d tid=%d ept=%llu depth=%llu pmap=%llx caller=%llx\n",
        timestamp, cpu, tid, self->epstart, self->epdepth,
        (uint64_t)arg0, (uint64_t)caller);
}

fbt:mach_kernel:pmap_flush_tlbs:entry
/self->epdepth == 0 && arg0 != 0 && *(uint64_t *)((uint64_t)arg0 + 0x40) == 0/
{
    self->epstart = timestamp;
    self->epdepth = 1;
    printf("EPT_ENTER t=%llu cpu=%d pid=%d tid=%d pmap=%llx start=%llx end=%llx options=%llx context=%llx caller=%llx\n",
        timestamp, cpu, pid, tid, (uint64_t)arg0, (uint64_t)arg1,
        (uint64_t)arg2, (uint64_t)arg3, (uint64_t)arg4, (uint64_t)caller);
}

/* Outer returns clear depth before the later nested-return predicate. */
fbt:mach_kernel:pmap_flush_tlbs:return
/self->epdepth == 1/
{
    printf("EPT_RETURN t=%llu cpu=%d pid=%d tid=%d ept=%llu ns=%llu offset=%llu pending_mp_depth=%llu\n",
        timestamp, cpu, pid, tid, self->epstart,
        timestamp - self->epstart, (uint64_t)arg0, self->mpdepth);
    self->epdepth = 0;
    self->epstart = 0;
    self->mpdepth = 0;
    self->mpstart = 0;
}

fbt:mach_kernel:pmap_flush_tlbs:return
/self->epdepth > 1/
{
    printf("EPT_NEST_RETURN t=%llu cpu=%d tid=%d ept=%llu depth=%llu offset=%llu\n",
        timestamp, cpu, tid, self->epstart, self->epdepth, (uint64_t)arg0);
    self->epdepth--;
}

/* Nested MP calls are balanced but never replace the outer timestamp. */
fbt:mach_kernel:mp_cpus_call1:entry
/self->mpdepth != 0/
{
    self->mpdepth++;
    printf("MP_NEST_ENTER t=%llu cpu=%d tid=%d ept=%llu mp=%llu depth=%llu callback=%llx caller=%llx\n",
        timestamp, cpu, tid, self->epstart, self->mpstart, self->mpdepth,
        (uint64_t)arg2, (uint64_t)caller);
}

/* Exact EPT callsite: callback 42a700 minus return site 42a489 = 0x277. */
fbt:mach_kernel:mp_cpus_call1:entry
/self->epdepth != 0 && self->mpdepth == 0 && arg1 == 1 &&
 (uint64_t)arg2 - (uint64_t)caller == 0x277/
{
    self->mpstart = timestamp;
    self->mpdepth = 1;
    printf("EPT_MP_ENTER t=%llu cpu=%d pid=%d tid=%d ept=%llu mask=%llx mode=%llu callback=%llx caller=%llx callback_arg0=%llx callback_arg1=%llx slide=%llx\n",
        timestamp, cpu, pid, tid, self->epstart, (uint64_t)arg0,
        (uint64_t)arg1, (uint64_t)arg2, (uint64_t)caller,
        (uint64_t)arg3, (uint64_t)arg4,
        (uint64_t)((uint64_t)arg2 - 0xffffff800042a700));
}

fbt:mach_kernel:mp_cpus_call1:return
/self->mpdepth == 1/
{
    printf("EPT_MP_RETURN t=%llu cpu=%d pid=%d tid=%d ept=%llu mp=%llu ns=%llu offset=%llu result=%llx\n",
        timestamp, cpu, pid, tid, self->epstart, self->mpstart,
        timestamp - self->mpstart, (uint64_t)arg0, (uint64_t)arg1);
    self->mpdepth = 0;
    self->mpstart = 0;
}

fbt:mach_kernel:mp_cpus_call1:return
/self->mpdepth > 1/
{
    printf("MP_NEST_RETURN t=%llu cpu=%d tid=%d ept=%llu mp=%llu depth=%llu offset=%llu result=%llx\n",
        timestamp, cpu, tid, self->epstart, self->mpstart, self->mpdepth,
        (uint64_t)arg0, (uint64_t)arg1);
    self->mpdepth--;
}

profile:::tick-1s
{
    printf("TICK t=%llu wall=%lld cpu=%d elapsed_ns=%llu\n",
        timestamp, walltimestamp, cpu, timestamp - started);
}

profile:::tick-1s
/timestamp - started >= 180000000000/
{
    exit(0);
}

dtrace:::ERROR
{
    printf("ERROR t=%llu cpu=%d epid=%llu action=%llu offset=%llu fault=%llu address=%llx\n",
        timestamp, cpu, (uint64_t)arg1, (uint64_t)arg2,
        (uint64_t)arg3, (uint64_t)arg4, (uint64_t)arg5);
}

dtrace:::END
{
    printf("END t=%llu wall=%lld\n", timestamp, walltimestamp);
}
