package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#include <Cocoa/Cocoa.h>

extern void probeFired(int id, int kind);
extern void runLoopMode(int id, char *mode);
extern void trayTitleObserved(int id, char *title);

// kind 0: the main dispatch queue, which is what systemtray_darwin.m uses.
static void probeDispatchQueue(int id) {
	dispatch_async(dispatch_get_main_queue(), ^{
		probeFired(id, 0);
	});
}

// kind 1: the main run loop in common modes, which is what
// pkg/application/mainthread_darwin.go uses since #6026.
static void probeRunLoop(int id) {
	CFRunLoopRef loop = CFRunLoopGetMain();
	CFRunLoopPerformBlock(loop, kCFRunLoopCommonModes, ^{
		// A nested menu or modal loop shows up as a non-default current mode,
		// which is how the transcript proves tracking was really active.
		CFStringRef mode = CFRunLoopCopyCurrentMode(CFRunLoopGetMain());
		runLoopMode(id, (char *)[(NSString *)mode UTF8String]);
		CFRelease(mode);
		probeFired(id, 1);
	});
	CFRunLoopWakeUp(loop);
}

// The status item button lives in an app-owned window, so its rendered title
// can be read back to distinguish a delivered update from a queued one.
static NSButton *findStatusBarButton(NSView *view) {
	if (view == nil) {
		return nil;
	}
	if ([view isKindOfClass:[NSButton class]]) {
		return (NSButton *)view;
	}
	for (NSView *child in [view subviews]) {
		NSButton *button = findStatusBarButton(child);
		if (button != nil) {
			return button;
		}
	}
	return nil;
}

static void observeTrayTitle(int id) {
	CFRunLoopRef loop = CFRunLoopGetMain();
	CFRunLoopPerformBlock(loop, kCFRunLoopCommonModes, ^{
		const char *found = "<no status item button>";
		for (NSWindow *window in [NSApp windows]) {
			NSButton *button = findStatusBarButton([window contentView]);
			if (button != nil) {
				found = [[button title] UTF8String];
				break;
			}
		}
		trayTitleObserved(id, (char *)found);
	});
	CFRunLoopWakeUp(loop);
}


// Menu tracking consumes events from the application event queue, so a posted
// Escape dismisses it without needing synthetic HID events or accessibility
// permission.
static void postEscape(void) {
	CFRunLoopRef loop = CFRunLoopGetMain();
	CFRunLoopPerformBlock(loop, kCFRunLoopCommonModes, ^{
		NSEvent *escape = [NSEvent keyEventWithType:NSEventTypeKeyDown
		                                   location:NSZeroPoint
		                              modifierFlags:0
		                                  timestamp:[[NSProcessInfo processInfo] systemUptime]
		                               windowNumber:0
		                                    context:nil
		                                 characters:@"\033"
		                charactersIgnoringModifiers:@"\033"
		                                  isARepeat:NO
		                                    keyCode:53];
		[NSApp postEvent:escape atStart:YES];
	});
	CFRunLoopWakeUp(loop);
}
*/
import "C"

import (
	"sync"
	"time"
)

var (
	probeMu    sync.Mutex
	probeStart = map[int]time.Time{}
)

// probe schedules one block on each delivery mechanism so the log shows which
// of the two reaches the main thread while a menu is tracking.
func probe(id int, observeTitle bool) {
	probeMu.Lock()
	probeStart[id] = time.Now()
	probeMu.Unlock()
	C.probeDispatchQueue(C.int(id))
	C.probeRunLoop(C.int(id))
	if observeTitle {
		C.observeTrayTitle(C.int(id))
	}
}

//export runLoopMode
func runLoopMode(id C.int, mode *C.char) {
	logf("mode  main run loop is in %s at tick %d", C.GoString(mode), int(id))
}

//export trayTitleObserved
func trayTitleObserved(id C.int, title *C.char) {
	logf("read  status item button title is %q at tick %d", C.GoString(title), int(id))
}

func dismissMenu() {
	C.postEscape()
}

//export probeFired
func probeFired(id C.int, kind C.int) {
	probeMu.Lock()
	started := probeStart[int(id)]
	probeMu.Unlock()
	name := "dispatch_async(main queue)"
	if kind == 1 {
		name = "CFRunLoopPerformBlock(common)"
	}
	logf("ran   %-30s scheduled with tick %d, %.3fs late", name, int(id), time.Since(started).Seconds())
}
