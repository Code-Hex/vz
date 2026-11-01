"""stackshot を発火せず、カーネルに記録された採取回数だけ読む。"""
import ctypes
import json
import time
import sys

library = ctypes.CDLL(None, use_errno=True)
call = library.sysctlbyname
call.argtypes = [ctypes.c_char_p, ctypes.c_void_p, ctypes.POINTER(ctypes.c_size_t), ctypes.c_void_p, ctypes.c_size_t]
call.restype = ctypes.c_int
fields = (ctypes.c_uint64 * 4)()
size = ctypes.c_size_t(ctypes.sizeof(fields))
class Timebase(ctypes.Structure):
    _fields_ = [('numer', ctypes.c_uint32), ('denom', ctypes.c_uint32)]

absolute = library.mach_absolute_time
absolute.argtypes = []
absolute.restype = ctypes.c_uint64
timebase = Timebase()
get_timebase = library.mach_timebase_info
get_timebase.argtypes = [ctypes.POINTER(Timebase)]
get_timebase.restype = ctypes.c_int
timebase_error = get_timebase(ctypes.byref(timebase))
before_absolute = absolute()
result = call(b'kern.stackshot_stats', ctypes.byref(fields), ctypes.byref(size), None, 0)
after_absolute = absolute()
record = dict(mach_absolute_before=before_absolute, mach_absolute_after=after_absolute,
    mach_timebase_numer=timebase.numer, mach_timebase_denom=timebase.denom,
    mach_timebase_errno=timebase_error, python_version=list(sys.version_info[:3]), monotonic_ns=time.monotonic_ns(), wall_ns=time.time_ns(), errno=ctypes.get_errno() if result else 0, size=size.value)
if result == 0 and size.value == 32:
    record.update(zip(('last_start', 'last_end', 'count', 'duration'), fields))
print(json.dumps(record))
raise SystemExit(0 if result == 0 and size.value == 32 else 1)
