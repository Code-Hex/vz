#import "Window.m"
#import "virtualization_view.h"
#import <unistd.h>

static void require(BOOL condition, NSString *message)
{
    if (!condition) {
        fprintf(stderr, "%s\n", message.UTF8String);
        exit(1);
    }
}

static VZVirtualMachine *newMachine(void)
{
    VZLinuxBootLoader *loader = [[[VZLinuxBootLoader alloc]
        initWithKernelURL:[NSURL fileURLWithPath:@"testdata/Image"]] autorelease];
    loader.initialRamdiskURL = [NSURL fileURLWithPath:@"testdata/initramfs.cpio.gz"];
    loader.commandLine = @"console=tty0 console=hvc0";
    VZVirtualMachineConfiguration *configuration = [[[VZVirtualMachineConfiguration alloc] init] autorelease];
    configuration.bootLoader = loader;
    configuration.CPUCount = 1;
    configuration.memorySize = 256 * 1024 * 1024;
    VZVirtioGraphicsDeviceConfiguration *graphics = [[[VZVirtioGraphicsDeviceConfiguration alloc] init] autorelease];
    graphics.scanouts = @[ [[[VZVirtioGraphicsScanoutConfiguration alloc]
        initWithWidthInPixels:640
               heightInPixels:480] autorelease] ];
    configuration.graphicsDevices = @[ graphics ];
    configuration.keyboards = @[ [[[VZUSBKeyboardConfiguration alloc] init] autorelease] ];
    configuration.pointingDevices = @[ [[[VZUSBScreenCoordinatePointingDeviceConfiguration alloc] init] autorelease] ];
    NSError *error = nil;
    require([configuration validateWithError:&error], [NSString stringWithFormat:@"Invalid VM configuration: %@", error]);
    return vz_newVZVirtualMachineWithDispatchQueue(configuration, 0, 0);
}

typedef void (^MachineOperation)(void (^)(NSError *));

static void operate(VZVirtualMachine *machine, MachineOperation operation)
{
    dispatch_semaphore_t completed = dispatch_semaphore_create(0);
    __block NSError *failure = nil;
    dispatch_sync(vz_objectQueue(machine), ^{
        operation(^(NSError *error) {
            failure = [error retain];
            dispatch_semaphore_signal(completed);
        });
    });
    require(dispatch_semaphore_wait(completed, dispatch_time(DISPATCH_TIME_NOW, 15 * NSEC_PER_SEC)) == 0,
        @"VM operation timed out");
    require(failure == nil, [NSString stringWithFormat:@"VM operation failed: %@", failure]);
    [failure release];
    dispatch_release(completed);
}

static VZVirtualMachineState state(VZVirtualMachine *machine)
{
    __block VZVirtualMachineState result;
    dispatch_sync(vz_objectQueue(machine), ^{
        result = machine.state;
    });
    return result;
}

static NSWindow *windowNamed(NSString *title)
{
    for (NSWindow *window in NSApp.windows) {
        if ([window.title isEqualToString:title]) {
            return window;
        }
    }
    return nil;
}

static VZVirtualMachineView *machineView(NSView *view)
{
    if ([view isKindOfClass:[VZVirtualMachineView class]]) {
        return (VZVirtualMachineView *)view;
    }
    for (NSView *child in view.subviews) {
        VZVirtualMachineView *found = machineView(child);
        if (found) {
            return found;
        }
    }
    return nil;
}

static NSUInteger liveObjects(NSHashTable *objects)
{
    @autoreleasepool {
        return objects.allObjects.count;
    }
}

static void later(void (^body)(void))
{
    [NSTimer scheduledTimerWithTimeInterval:0.3
                                    repeats:NO
                                      block:^(NSTimer *timer) {
                                          body();
                                      }];
}

typedef NS_ENUM(NSUInteger, Scenario) {
    CloseFirst,
    CloseSecond,
    StopFirst,
    Quit,
    SingleWindow,
    AddLater,
    AddOnMain,
};

static void checkWindowMenuState(NSWindow *first, NSWindow *second)
{
    NSMenu *menu = NSApp.mainMenu.itemArray.firstObject.submenu;
    NSInteger index = [menu indexOfItemWithTarget:nil andAction:@selector(toggleCapturesSystemKeys:)];
    require(index >= 0, @"System key capture menu item is missing");
    NSMenuItem *item = [menu itemAtIndex:index];
    [first makeKeyAndOrderFront:nil];
    [menu update];
    require(item.state == NSControlStateValueOn, @"First window does not initially capture system keys");
    [menu performActionForItemAtIndex:index];
    require(item.state == NSControlStateValueOff, @"First window did not disable system key capture");
    [second makeKeyAndOrderFront:nil];
    [menu update];
    require(item.state == NSControlStateValueOn, @"Menu retained the previous window's capture state");
    [menu performActionForItemAtIndex:index];
    require(item.state == NSControlStateValueOff, @"Second window did not disable system key capture");
    [first makeKeyAndOrderFront:nil];
    [menu update];
    require(item.state == NSControlStateValueOff, @"First window's capture state changed with the second window");
    [menu performActionForItemAtIndex:index];
    require(item.state == NSControlStateValueOn, @"First window did not re-enable system key capture");
    [second makeKeyAndOrderFront:nil];
    [menu update];
    require(item.state == NSControlStateValueOff, @"Menu does not reflect the focused window's capture state");
}

static void runScenario(Scenario scenario, VZVirtualMachine *first, VZVirtualMachine *second)
{
    NSHashTable *controllers = [[NSHashTable weakObjectsHashTable] retain];
    NSHashTable *windows = [[NSHashTable weakObjectsHashTable] retain];
    NSHashTable *views = [[NSHashTable weakObjectsHashTable] retain];
    @autoreleasepool {
        void *application = vz_newGraphicApplication();
        vz_addGraphicWindow(application, first, 640, 480, "First VM", true);
        if (scenario != SingleWindow && scenario != AddLater && scenario != AddOnMain) {
            vz_addGraphicWindow(application, second, 640, 480, "Second VM", true);
        }
        __block BOOL completed = NO;
        void (^checkWindows)(void) = ^{
            NSWindow *firstWindow = windowNamed(@"First VM");
            NSWindow *secondWindow = windowNamed(@"Second VM");
            require(firstWindow.visible, @"First VM window is not visible");
            [controllers addObject:firstWindow.delegate];
            [windows addObject:firstWindow];
            require(machineView(firstWindow.contentView) != nil, @"First VM view is missing");
            [views addObject:machineView(firstWindow.contentView)];
            if (scenario != SingleWindow) {
                require(secondWindow.visible, @"Second VM window is not visible");
                [controllers addObject:secondWindow.delegate];
                [windows addObject:secondWindow];
                require(machineView(secondWindow.contentView) != nil, @"Second VM view is missing");
                [views addObject:machineView(secondWindow.contentView)];
            }
            if (scenario == CloseFirst) {
                checkWindowMenuState(firstWindow, secondWindow);
                require(vz_runGraphicApplication(application) == VZGraphicApplicationDuplicateMachine, @"Duplicate application windows were accepted");
            }
            if (scenario == Quit) {
                completed = YES;
                [NSApp terminate:nil];
                return;
            }
            if (scenario == SingleWindow) {
                completed = YES;
                [firstWindow performClose:nil];
                return;
            }
            VZVirtualMachine *remaining = scenario == CloseSecond ? first : second;
            NSString *remainingTitle = scenario == CloseSecond ? @"First VM" : @"Second VM";
            NSString *closedTitle = scenario == CloseSecond ? @"Second VM" : @"First VM";
            if (scenario == StopFirst) {
                operate(first, ^(void (^done)(NSError *)) {
                    [first stopWithCompletionHandler:done];
                });
            } else {
                [windowNamed(closedTitle) performClose:nil];
            }
            later(^{
                require(state(remaining) == VZVirtualMachineStateRunning, @"Remaining VM stopped running");
                require(liveObjects(controllers) == 1, @"Closed window controller is retained while another VM runs");
                require(!windowNamed(closedTitle).visible, @"Closed VM window remains visible");
                require(windowNamed(remainingTitle).visible, @"Closing one VM hid the other window");
                operate(remaining, ^(void (^done)(NSError *)) {
                    [remaining pauseWithCompletionHandler:done];
                });
                require(state(remaining) == VZVirtualMachineStatePaused, @"Remaining VM did not pause");
                operate(remaining, ^(void (^done)(NSError *)) {
                    [remaining resumeWithCompletionHandler:done];
                });
                require(state(remaining) == VZVirtualMachineStateRunning, @"Remaining VM did not resume");
                completed = YES;
                [windowNamed(remainingTitle) performClose:nil];
            });
        };
        later(^{
            if (scenario == AddOnMain) {
                void *addition = vz_newGraphicApplication();
                vz_addGraphicWindow(addition, second, 640, 480, "Second VM", true);
                require(vz_runGraphicApplication(addition) == VZGraphicApplicationSuccess, @"Adding a window on the main thread failed");
                vz_freeGraphicApplication(addition);
            }
            if (scenario != AddLater) {
                checkWindows();
                return;
            }
            dispatch_async(dispatch_get_global_queue(QOS_CLASS_DEFAULT, 0), ^{
                @autoreleasepool {
                    void *addition = vz_newGraphicApplication();
                    vz_addGraphicWindow(addition, second, 640, 480, "Second VM", true);
                    require(vz_runGraphicApplication(addition) == VZGraphicApplicationSuccess, @"Adding a window from a background thread failed");
                    require(vz_runGraphicApplication(addition) == VZGraphicApplicationDuplicateMachine, @"Duplicate window was accepted");
                    vz_freeGraphicApplication(addition);
                    dispatch_async(dispatch_get_main_queue(), checkWindows);
                }
            });
        });
        require(vz_runGraphicApplication(application) == VZGraphicApplicationSuccess, @"Application run failed");
        require(completed, @"Event loop returned before the remaining window closed");
        require(!windowNamed(@"First VM").visible && !windowNamed(@"Second VM").visible,
            @"Application left a VM window visible after returning");
        vz_freeGraphicApplication(application);
    }
    NSDate *deadline = [NSDate dateWithTimeIntervalSinceNow:2];
    while ((liveObjects(controllers) || liveObjects(windows) || liveObjects(views)) && deadline.timeIntervalSinceNow > 0) {
        @autoreleasepool {
            [[NSRunLoop mainRunLoop] runUntilDate:[NSDate dateWithTimeIntervalSinceNow:0.01]];
        }
    }
    require(!liveObjects(controllers), @"Closed window controller is still retained");
    require(!liveObjects(windows), @"Closed window is still retained");
    require(!liveObjects(views), @"Closed VM view is still retained");
    [controllers release];
    [windows release];
    [views release];
}

static void closeWithPendingAddition(VZVirtualMachine *first, VZVirtualMachine *second, BOOL quit)
{
    NSHashTable *objects = [[NSHashTable weakObjectsHashTable] retain];
    dispatch_semaphore_t completed = dispatch_semaphore_create(0);
    @autoreleasepool {
        void *application = vz_newGraphicApplication();
        vz_addGraphicWindow(application, first, 640, 480, "First VM", true);
        later(^{
            NSWindow *firstWindow = windowNamed(@"First VM");
            [objects addObject:firstWindow];
            [objects addObject:firstWindow.delegate];
            [objects addObject:machineView(firstWindow.contentView)];
            dispatch_async(dispatch_get_global_queue(QOS_CLASS_DEFAULT, 0), ^{
                @autoreleasepool {
                    void *addition = vz_newGraphicApplication();
                    vz_addGraphicWindow(addition, second, 640, 480, "Second VM", true);
                    VZGraphicApplicationResult expected = quit ? VZGraphicApplicationStopping : VZGraphicApplicationSuccess;
                    require(vz_runGraphicApplication(addition) == expected, @"Unexpected result for an admitted window");
                    vz_freeGraphicApplication(addition);
                    if (!quit) {
                        dispatch_sync(dispatch_get_main_queue(), ^{
                            NSWindow *window = windowNamed(@"Second VM");
                            require(window.visible, @"Admitted window is not visible");
                            require(!windowNamed(@"First VM").visible, @"First window did not close");
                            [objects addObject:window];
                            [objects addObject:window.delegate];
                            [objects addObject:machineView(window.contentView)];
                            [window performClose:nil];
                        });
                    }
                    dispatch_semaphore_signal(completed);
                }
            });
            NSDate *deadline = [NSDate dateWithTimeIntervalSinceNow:2];
            BOOL pending = NO;
            while (!pending && deadline.timeIntervalSinceNow > 0) {
                pthread_mutex_lock(&applicationLock);
                pending = activeApplication->pendingAdditions != 0;
                pthread_mutex_unlock(&applicationLock);
                if (!pending) {
                    usleep(1000);
                }
            }
            require(pending, @"Background window request was not admitted");
            if (quit) {
                [NSApp terminate:nil];
            } else {
                [firstWindow performClose:nil];
            }
        });
        require(vz_runGraphicApplication(application) == VZGraphicApplicationSuccess, @"Application run failed");
        require(dispatch_semaphore_wait(completed, dispatch_time(DISPATCH_TIME_NOW, NSEC_PER_SEC)) == 0,
            @"Last close left the background window request blocked");
        require(!windowNamed(@"Second VM").visible, @"Second window remained visible");
        vz_freeGraphicApplication(application);
    }
    NSDate *deadline = [NSDate dateWithTimeIntervalSinceNow:2];
    while (liveObjects(objects) && deadline.timeIntervalSinceNow > 0) {
        @autoreleasepool {
            [[NSRunLoop mainRunLoop] runUntilDate:[NSDate dateWithTimeIntervalSinceNow:0.01]];
        }
    }
    require(liveObjects(objects) == 0, @"Last close/addition race retained controller, window, or view");
    [objects release];
    dispatch_release(completed);
}

int main(void)
{
    @autoreleasepool {
        if (@available(macOS 13, *)) {
            alarm(90);
            require([[NSFileManager defaultManager] fileExistsAtPath:@"testdata/Image"], @"Run make download_kernel first");
            VZVirtualMachine *first = newMachine();
            VZVirtualMachine *second = newMachine();
            operate(first, ^(void (^done)(NSError *)) {
                [first startWithCompletionHandler:done];
            });
            operate(second, ^(void (^done)(NSError *)) {
                [second startWithCompletionHandler:done];
            });
            void *offMainApplication = vz_newGraphicApplication();
            vz_addGraphicWindow(offMainApplication, first, 640, 480, "Off main", false);
            dispatch_semaphore_t checkedThread = dispatch_semaphore_create(0);
            dispatch_async(dispatch_get_global_queue(QOS_CLASS_DEFAULT, 0), ^{
                require(vz_runGraphicApplication(offMainApplication) == VZGraphicApplicationNotMainThread, @"Off-main application run was accepted");
                dispatch_semaphore_signal(checkedThread);
            });
            require(dispatch_semaphore_wait(checkedThread, dispatch_time(DISPATCH_TIME_NOW, NSEC_PER_SEC)) == 0,
                @"Off-main check timed out");
            dispatch_release(checkedThread);
            vz_freeGraphicApplication(offMainApplication);
            runScenario(CloseFirst, first, second);
            runScenario(CloseSecond, first, second);
            runScenario(AddLater, first, second);
            runScenario(AddOnMain, first, second);
            closeWithPendingAddition(first, second, NO);
            closeWithPendingAddition(first, second, YES);
            runScenario(Quit, first, second);
            runScenario(SingleWindow, first, second);
            runScenario(StopFirst, first, second);
            require(state(first) == VZVirtualMachineStateStopped, @"First VM did not stop");
            operate(second, ^(void (^done)(NSError *)) {
                [second stopWithCompletionHandler:done];
            });
            vz_releaseObject(first);
            vz_releaseObject(second);
            vz_drain();
            alarm(0);
            puts("Multiple VM window tests passed");
        }
    }
    return 0;
}
