//go:build darwin

package appui

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework AppKit

#import <AppKit/AppKit.h>

void containerwayMaximizeWindow(void *ptr) {
	if (!ptr) { return; }
	NSWindow *win = (NSWindow *)ptr;
	NSScreen *screen = [win screen];
	if (!screen) { return; }
	NSRect frame = [screen visibleFrame];
	[win setFrame:frame display:YES];
}
*/
import "C"

import (
	"unsafe"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver"
)

func maximizeMainWindow(w fyne.Window) {
	if w == nil {
		return
	}
	nw, ok := w.(driver.NativeWindow)
	if !ok {
		return
	}
	nw.RunNative(func(ctx any) {
		c, ok := ctx.(driver.MacWindowContext)
		if !ok || c.NSWindow == 0 {
			return
		}
		C.containerwayMaximizeWindow(unsafe.Pointer(uintptr(c.NSWindow)))
	})
}
