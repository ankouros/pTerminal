//go:build linux

package ui

/*
#cgo pkg-config: gtk+-3.0
#include <gtk/gtk.h>

extern gboolean windowDeleteEventCallback();

static gboolean window_delete_event_cb(GtkWidget *widget, GdkEvent *event, gpointer user_data) {
    return windowDeleteEventCallback();
}

static void install_window_delete_handler(GtkWidget *window) {
    if (!window) return;
    g_signal_connect(window, "delete-event", G_CALLBACK(window_delete_event_cb), NULL);
    gtk_widget_hide_on_delete(window);
}

*/
import "C"

import (
	"unsafe"

	"github.com/webview/webview_go"
)

func hideGtkWindow(w webview.WebView) {
	if w == nil {
		return
	}
	C.gtk_widget_hide((*C.GtkWidget)(w.Window()))
}

func showGtkWindow(w webview.WebView) {
	if w == nil {
		return
	}
	C.gtk_widget_show_all((*C.GtkWidget)(w.Window()))
}

func installWindowCloseHandler(w webview.WebView) {
	if w == nil {
		return
	}
	C.install_window_delete_handler((*C.GtkWidget)(unsafe.Pointer(w.Window())))
}

//export windowDeleteEventCallback
func windowDeleteEventCallback() C.gboolean {
	if trayWindow == nil {
		return C.FALSE
	}
	trayWindow.hideFromTray()
	return C.TRUE
}
