#import <Cocoa/Cocoa.h>
#import <Virtualization/Virtualization.h>
bool vz_viewIsMainThread(void);
void *vz_viewNew(void *machine);
void vz_viewRelease(void *view);
bool vz_viewCapturesKeys(void *view);
void vz_viewSetCapturesKeys(void *view, bool value);
bool vz_viewAutomaticallyReconfigures(void *view);
void vz_viewSetAutomaticallyReconfigures(void *view, bool value);

void *vz_viewBorrowPoolNew(void);
void vz_viewBorrowPoolDrain(void *pool);
