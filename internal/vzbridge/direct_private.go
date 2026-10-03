//go:build darwin

package vzbridge

import (
	"unsafe"

	runtimeobjc "github.com/ebitengine/purego/objc"
)

type privateRuntime struct {
	classMethod         func(unsafe.Pointer, runtimeobjc.SEL) unsafe.Pointer
	methodReturn        func(unsafe.Pointer) unsafe.Pointer
	methodArgument      func(unsafe.Pointer, uint32) unsafe.Pointer
	methodArgumentCount func(unsafe.Pointer) uint32
	free                func(unsafe.Pointer)
}

var privateRuntimeBinding lazyBinding[privateRuntime]

func loadPrivateRuntime() privateRuntime {
	return privateRuntimeBinding.get(func() privateRuntime {
		return privateRuntime{
			classMethod:         directSymbol[func(unsafe.Pointer, runtimeobjc.SEL) unsafe.Pointer]("class_getClassMethod"),
			methodReturn:        directSymbol[func(unsafe.Pointer) unsafe.Pointer]("method_copyReturnType"),
			methodArgument:      directSymbol[func(unsafe.Pointer, uint32) unsafe.Pointer]("method_copyArgumentType"),
			methodArgumentCount: directSymbol[func(unsafe.Pointer) uint32]("method_getNumberOfArguments"),
			free:                directSymbol[func(unsafe.Pointer)]("free"),
		}
	})
}

func privateEncodingMatches(pointer unsafe.Pointer, expected string) bool {
	if pointer == nil {
		return false
	}
	defer loadPrivateRuntime().free(pointer)
	for i := 0; i < len(expected); i++ {
		if *(*byte)(unsafe.Add(pointer, i)) != expected[i] {
			return false
		}
	}
	return *(*byte)(unsafe.Add(pointer, len(expected))) == 0
}

func privateAvailable(receiver unsafe.Pointer, className string, selector runtimeobjc.SEL, classMethod bool, result string, args []string) bool {
	expected := directGetClass(className)
	if receiver == nil || expected == nil {
		return false
	}
	api := loadPrivateRuntime()
	var method unsafe.Pointer
	if classMethod {
		if receiver != expected {
			return false
		}
		method = api.classMethod(expected, selector)
	} else {
		actual := directObjectClass(receiver)
		class := actual
		for class != nil && class != expected {
			class = directSuperclass(class)
		}
		if class == nil {
			return false
		}
		method = directInstanceMethod(actual, selector)
	}
	if method == nil || api.methodArgumentCount(method) != uint32(len(args)) {
		return false
	}
	if !privateEncodingMatches(api.methodReturn(method), result) {
		return false
	}
	for i, want := range args {
		if !privateEncodingMatches(api.methodArgument(method, uint32(i)), want) {
			return false
		}
	}
	return true
}

func UnsafePrivateAllocate(className string) unsafe.Pointer {
	var result unsafe.Pointer
	onQueue(func() {
		class := directGetClass(className)
		if privateAvailable(class, className, runtimeobjc.RegisterName("alloc"), true, "@", []string{"@", ":"}) {
			result = directAllocate(class)
		}
	})
	return result
}
