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

void containerwayRestoreNormalWindow(void *ptr) {
	if (!ptr) { return; }
	NSWindow *win = (NSWindow *)ptr;
	if ([win isZoomed]) {
		[win zoom:nil];
	}
}
*/
import "C"

import (
	"unsafe"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver"
)

// maximizeMainWindow executa parte da logica deste modulo.
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

// restoreNormalMainWindow executa parte da logica deste modulo.
func restoreNormalMainWindow(w fyne.Window) {
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
		C.containerwayRestoreNormalWindow(unsafe.Pointer(uintptr(c.NSWindow)))
	})
}
