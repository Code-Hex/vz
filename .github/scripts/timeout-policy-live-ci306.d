#pragma D option quiet
dtrace:::BEGIN
{
    printf("META wall_ns=%llu mono_ns=%llu\n", (uint64_t)walltimestamp, (uint64_t)timestamp);
    printf("SYMBOL mp_cpus_call1 address=0x%llx\n", (uint64_t)&mach_kernel`mp_cpus_call1);
    printf("SYMBOL pmap_flush_tlbs address=0x%llx\n", (uint64_t)&mach_kernel`pmap_flush_tlbs);
    printf("VALUE LockTimeOutUsec address=0x%llx width=32 value=%llu\n", (uint64_t)&mach_kernel`LockTimeOutUsec, (uint64_t)*(uint32_t *)&mach_kernel`LockTimeOutUsec);
    printf("VALUE TLBTimeOut address=0x%llx width=64 value=%llu\n", (uint64_t)&mach_kernel`TLBTimeOut, (uint64_t)*(uint64_t *)&mach_kernel`TLBTimeOut);
    printf("VALUE debugger_entry_time address=0x%llx width=64 value=%llu\n", (uint64_t)&mach_kernel`debugger_entry_time, (uint64_t)*(uint64_t *)&mach_kernel`debugger_entry_time);
    printf("VALUE LockTimeOutTSC address=0x%llx width=64 value=%llu\n", (uint64_t)&mach_kernel`LockTimeOutTSC, (uint64_t)*(uint64_t *)&mach_kernel`LockTimeOutTSC);
    printf("VALUE debugger_exit_time address=0x%llx width=64 value=%llu\n", (uint64_t)&mach_kernel`debugger_exit_time, (uint64_t)*(uint64_t *)&mach_kernel`debugger_exit_time);
    printf("VALUE LastDebuggerEntryAllowance address=0x%llx width=64 value=%llu\n", (uint64_t)&mach_kernel`LastDebuggerEntryAllowance, (uint64_t)*(uint64_t *)&mach_kernel`LastDebuggerEntryAllowance);
    printf("VALUE lck_spinlock_timeout_in_progress address=0x%llx width=64 value=%llu\n", (uint64_t)&mach_kernel`lck_spinlock_timeout_in_progress, (uint64_t)*(uint64_t *)&mach_kernel`lck_spinlock_timeout_in_progress);
    printf("VALUE pmap_tlb_flush_timeout address=0x%llx width=32 value=%llu\n", (uint64_t)&mach_kernel`pmap_tlb_flush_timeout, (uint64_t)*(uint32_t *)&mach_kernel`pmap_tlb_flush_timeout);
    printf("VALUE tscFreq address=0x%llx width=64 value=%llu\n", (uint64_t)&mach_kernel`tscFreq, (uint64_t)*(uint64_t *)&mach_kernel`tscFreq);
    printf("VALUE virtualized address=0x%llx width=32 value=%llu\n", (uint64_t)&mach_kernel`virtualized, (uint64_t)*(uint32_t *)&mach_kernel`virtualized);
    printf("VALUE LockTimeOut address=0x%llx width=64 value=%llu\n", (uint64_t)&mach_kernel`LockTimeOut, (uint64_t)*(uint64_t *)&mach_kernel`LockTimeOut);
    printf("VALUE disable_serial_output address=0x%llx width=8 value=%llu\n", (uint64_t)&mach_kernel`disable_serial_output, (uint64_t)*(uint8_t *)&mach_kernel`disable_serial_output);
    printf("VALUE debugger_current_op address=0x%llx width=32 value=%llu\n", (uint64_t)&mach_kernel`debugger_current_op, (uint64_t)*(uint32_t *)&mach_kernel`debugger_current_op);
    printf("VALUE vm_kernel_slide address=0x%llx width=64 value=%llu\n", (uint64_t)&mach_kernel`vm_kernel_slide, (uint64_t)*(uint64_t *)&mach_kernel`vm_kernel_slide);
    printf("COMPLETE\n");
    exit(0);
}
dtrace:::ERROR { printf("ERROR code=%d address=0x%llx\n", arg4, (uint64_t)arg5); exit(2); }
