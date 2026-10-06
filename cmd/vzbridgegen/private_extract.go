package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

type privateMetadata struct {
	Architecture string
	OSVersion    string
	Methods      []privateMethod
}
type privateMethod struct {
	Class       string
	Image       string
	Selector    string
	ClassMethod bool
	Result      string
	Arguments   []string
}

func discoverPrivate(temp string) (privateMetadata, error) {
	var metadata privateMetadata
	directory, err := os.MkdirTemp(temp, "vz-private-")
	if err != nil {
		return metadata, err
	}
	defer os.RemoveAll(directory)
	source := filepath.Join(directory, "inspect.m")
	binary := filepath.Join(directory, "inspect")
	if err = os.WriteFile(source, []byte(privateInspector), 0600); err != nil {
		return metadata, err
	}
	if output, err := exec.Command("xcrun", "clang", "-fobjc-arc", "-framework", "Foundation", "-framework", "Virtualization", source, "-o", binary).CombinedOutput(); err != nil {
		return metadata, fmt.Errorf("compile runtime inspector: %w: %s", err, output)
	}
	output, err := exec.Command(binary).Output()
	if err != nil {
		return metadata, fmt.Errorf("inspect runtime: %w", err)
	}
	if err = json.Unmarshal(output, &metadata); err != nil {
		return metadata, fmt.Errorf("decode runtime metadata: %w", err)
	}
	if metadata.Architecture != runtime.GOARCH {
		return metadata, fmt.Errorf("runtime architecture %q differs from generator host %s", metadata.Architecture, runtime.GOARCH)
	}
	return metadata, nil
}

const privateInspector = `
#import <Foundation/Foundation.h>
#import <Virtualization/Virtualization.h>
#import <objc/runtime.h>

int main(void) {
 @autoreleasepool {
  const char *image = class_getImageName([VZVirtualMachineConfiguration class]);
  if (image == NULL) return 1;
  unsigned count = 0;
  const char **names = objc_copyClassNamesForImage(image, &count);
  NSMutableArray *methods = [NSMutableArray array];
  for (unsigned i = 0; i < count; i++) {
   Class cls = objc_getClass(names[i]);
   for (int classMethod = 0; classMethod < 2; classMethod++) {
    unsigned methodCount = 0;
    Method *list = class_copyMethodList(classMethod ? object_getClass(cls) : cls, &methodCount);
    for (unsigned j = 0; j < methodCount; j++) {
     Method method = list[j];
     char *result = method_copyReturnType(method);
     NSMutableArray *arguments = [NSMutableArray array];
     for (unsigned k = 0; k < method_getNumberOfArguments(method); k++) {
      char *argument = method_copyArgumentType(method, k);
      [arguments addObject:argument ? @(argument) : @"?"];
      free(argument);
     }
     [methods addObject:@{@"Class":@(names[i]), @"Image":@(image), @"Selector":@(sel_getName(method_getName(method))), @"ClassMethod":classMethod ? @YES : @NO, @"Result":result ? @(result) : @"?", @"Arguments":arguments}];
     free(result);
    }
    free(list);
   }
  }
  free(names);
#if defined(__arm64__)
  NSString *architecture = @"arm64";
#elif defined(__x86_64__)
  NSString *architecture = @"amd64";
#else
#error Unsupported architecture
#endif
  NSDictionary *metadata = @{@"Architecture":architecture, @"OSVersion":NSProcessInfo.processInfo.operatingSystemVersionString, @"Methods":methods};
  NSError *error = nil;
  NSData *data = [NSJSONSerialization dataWithJSONObject:metadata options:0 error:&error];
  if (data == nil) return 1;
  [NSFileHandle.fileHandleWithStandardOutput writeData:data];
 }
 return 0;
}
`
