//
//  virtualization_view.h
//
//  Created by codehex.
//

#pragma once

#import <Foundation/Foundation.h>
#import <Cocoa/Cocoa.h>
#import <Virtualization/Virtualization.h>

// @interface CanBecomeKeyWindow : NSWindow
// @property(readonly) BOOL canBecomeKeyWindow;
// @end

// @interface GraphicsAppTitlebarViewController : NSTitlebarAccessoryViewController
// - (instancetype)initWithVirtualMachine:(VZVirtualMachine *)virtualMachine
//     windowWidth:(CGFloat)windowWidth
//     windowHeight:(CGFloat)windowHeight;
// @end

@interface AppDelegate : NSObject <NSApplicationDelegate, NSWindowDelegate, VZVirtualMachineDelegate>
- (instancetype)initWithVirtualMachine:(VZVirtualMachine *)virtualMachine
    windowWidth:(CGFloat)windowWidth
    windowHeight:(CGFloat)windowHeight;
@end