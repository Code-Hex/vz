#include <stdint.h>
int vzGUIGioMountView(uintptr_t host, void *child);

void vzGUIGioFocusView(uintptr_t host, void *child);

void vzGUIGioRestoreHostFocus(uintptr_t host);

int vzGUIGioObserveWindowClose(uintptr_t host, uintptr_t callback);

void vzGUIGioScheduleHostClose(uintptr_t host);
