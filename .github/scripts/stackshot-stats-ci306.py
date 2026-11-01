"""stackshot を発火せず、カーネルに記録された採取回数だけ読む。"""
import ctypes
import json
import time

library = ctypes.CDLL(None, use_errno=True)
call = library.sysctlbyname
call.argtypes = [ctypes.c_char_p, ctypes.c_void_p, ctypes.POINTER(ctypes.c_size_t), ctypes.c_void_p, ctypes.c_size_t]
call.restype = ctypes.c_int
fields = (ctypes.c_uint64 * 4)()
size = ctypes.c_size_t(ctypes.sizeof(fields))
result = call(b'kern.stackshot_stats', ctypes.byref(fields), ctypes.byref(size), None, 0)
record = dict(monotonic_ns=time.monotonic_ns(), wall_ns=time.time_ns(), errno=ctypes.get_errno() if result else 0, size=size.value)
if result == 0 and size.value == 32:
    record.update(zip(('last_start', 'last_end', 'count', 'duration'), fields))
print(json.dumps(record))
raise SystemExit(0 if result == 0 and size.value == 32 else 1)
