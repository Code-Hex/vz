package objc

/*
#cgo darwin CFLAGS: -x objective-c
#cgo darwin LDFLAGS: -lobjc -framework Foundation
#import <Foundation/Foundation.h>

const char *getNSErrorLocalizedDescription(void *err)
{
	NSString *ld = (NSString *)[(NSError *)err localizedDescription];
	return [ld UTF8String];
}

const char *getNSErrorDomain(void *err)
{
	NSString *domain = (NSString *)[(NSError *)err domain];
	return [domain UTF8String];
}

const char *getNSErrorUserInfo(void *err)
{
	NSDictionary<NSErrorUserInfoKey, id> *ui = [(NSError *)err userInfo];
	NSString *uis = [NSString stringWithFormat:@"%@", ui];
	return [uis UTF8String];
}

NSInteger getNSErrorCode(void *err)
{
	return (NSInteger)[(NSError *)err code];
}

typedef struct NSErrorFlat {
	const char *domain;
    const char *localizedDescription;
	const char *userinfo;
    int code;
} NSErrorFlat;

NSErrorFlat convertNSError2Flat(void *err)
{
	NSErrorFlat ret;
	ret.domain = getNSErrorDomain(err);
	ret.localizedDescription = getNSErrorLocalizedDescription(err);
	ret.userinfo = getNSErrorUserInfo(err);
	ret.code = (int)getNSErrorCode(err);

	return ret;
}

void *makeNSMutableArray(unsigned long cap)
{
	return [[NSMutableArray alloc] initWithCapacity:(NSUInteger)cap];
}

void addNSMutableArrayVal(void *ary, void *val)
{
	[(NSMutableArray *)ary addObject:(NSObject *)val];
}

void *makeNSMutableDictionary()
{
	return [[NSMutableDictionary alloc] init];
}

void insertNSMutableDictionary(void *dict, char *key, void *val)
{
	NSString *nskey = [NSString stringWithUTF8String: key];
	[(NSMutableDictionary *)dict setValue:(NSObject *)val forKey:nskey];
}

void *newNSError()
{
	NSError *err = nil;
	return err;
}

bool hasError(void *err)
{
	return (NSError *)err != nil;
}

void releaseNSObject(void* o)
{
	@autoreleasepool {
		[(NSObject*)o release];
	}
}

static inline void releaseDispatch(void *queue)
{
	dispatch_release((dispatch_queue_t)queue);
}

int getNSArrayCount(void *ptr)
{
	return (int)[(NSArray*)ptr count];
}

void* getNSArrayItem(void *ptr, int i)
{
	NSArray *arr = (NSArray *)ptr;
	return [arr objectAtIndex:i];
}

const char *getUUID()
{
	NSString *uuid = [[NSUUID UUID] UUIDString];
	return [uuid UTF8String];
}
*/
import "C"
import (
	"fmt"
	"runtime"
	"unsafe"
)

// ReleaseDispatch releases allocated dispatch_queue_t
func ReleaseDispatch(p unsafe.Pointer) {
	C.releaseDispatch(p)
}

// Pointer indicates any pointers which are allocated in objective-c world.
type Pointer struct {
	_ptr unsafe.Pointer
}

// NewPointer creates a new Pointer for objc
func NewPointer(p unsafe.Pointer) *Pointer {
	return &Pointer{_ptr: p}
}

// release releases allocated resources in objective-c world.
func (p *Pointer) release() {
	C.releaseNSObject(p._ptr)
	runtime.KeepAlive(p)
}

// Ptr returns raw pointer.
func (o *Pointer) ptr() unsafe.Pointer {
	if o == nil {
		return nil
	}
	return o._ptr
}

// NSObject indicates NSObject
type NSObject interface {
	ptr() unsafe.Pointer
	release()
}

// Release releases allocated resources in objective-c world.
func Release(o NSObject) {
	o.release()
}

// Ptr returns unsafe.Pointer of the NSObject
func Ptr(o NSObject) unsafe.Pointer {
	return o.ptr()
}

// NSArray indicates NSArray
type NSArray struct {
	*Pointer
}

// NewNSArray creates a new NSArray from pointer.
func NewNSArray(p unsafe.Pointer) *NSArray {
	return &NSArray{NewPointer(p)}
}

// ToPointerSlice method returns slice of the obj-c object as unsafe.Pointer.
func (n *NSArray) ToPointerSlice() []unsafe.Pointer {
	count := int(C.getNSArrayCount(n.ptr()))
	ret := make([]unsafe.Pointer, count)
	for i := 0; i < count; i++ {
		ret[i] = C.getNSArrayItem(n.ptr(), C.int(i))
	}
	return ret
}

// NSError indicates NSError.
type NSError struct {
	Domain               string
	Code                 int
	LocalizedDescription string
	UserInfo             string
	Pointer
}

// NewNSErrorAsNil makes nil NSError in objective-c world.
func NewNSErrorAsNil() *Pointer {
	return &Pointer{
		_ptr: unsafe.Pointer(C.newNSError()),
	}
}

// HasNSError checks passed pointer is NSError or not.
func HasNSError(nserrPtr unsafe.Pointer) bool {
	return (bool)(C.hasError(nserrPtr))
}

func (n *NSError) Error() string {
	if n == nil {
		return "<nil>"
	}
	return fmt.Sprintf(
		"Error Domain=%s Code=%d Description=%q UserInfo=%s",
		n.Domain,
		n.Code,
		n.LocalizedDescription,
		n.UserInfo,
	)
}

func NewNSError(p unsafe.Pointer) *NSError {
	if !HasNSError(p) {
		return nil
	}
	nsError := C.convertNSError2Flat(p)
	return &NSError{
		Domain:               (*char)(nsError.domain).String(),
		Code:                 int((nsError.code)),
		LocalizedDescription: (*char)(nsError.localizedDescription).String(),
		UserInfo:             (*char)(nsError.userinfo).String(), // NOTE(codehex): maybe we can convert to map[string]interface{}
	}
}

// ConvertToNSMutableArray converts to NSMutableArray from NSObject slice in Go world.
func ConvertToNSMutableArray(s []NSObject) *Pointer {
	ln := len(s)
	ary := C.makeNSMutableArray(C.ulong(ln))
	for _, v := range s {
		C.addNSMutableArrayVal(ary, v.ptr())
	}
	p := NewPointer(ary)
	runtime.SetFinalizer(p, func(self *Pointer) {
		self.release()
	})
	return p
}

// ConvertToNSMutableDictionary converts to NSMutableDictionary from map[string]NSObject in Go world.
func ConvertToNSMutableDictionary(d map[string]NSObject) *Pointer {
	dict := C.makeNSMutableDictionary()
	for key, value := range d {
		cs := charWithGoString(key)
		C.insertNSMutableDictionary(dict, cs.CString(), value.ptr())
		cs.Free()
	}
	p := NewPointer(dict)
	runtime.SetFinalizer(p, func(self *Pointer) {
		self.release()
	})
	return p
}

func GetUUID() *C.char {
	return C.getUUID()
}

// CharWithGoString makes *Char which is *C.Char wrapper from Go string.
func charWithGoString(s string) *char {
	return (*char)(unsafe.Pointer(C.CString(s)))
}

// Char is a wrapper of C.char
type char C.char

// CString converts *C.char from *Char
func (c *char) CString() *C.char {
	return (*C.char)(c)
}

// String converts Go string from *Char
func (c *char) String() string {
	return C.GoString((*C.char)(c))
}

// Free frees allocated *C.char in Go code
func (c *char) Free() {
	C.free(unsafe.Pointer(c))
}
