//go:build linux

package ui

/*
#cgo pkg-config: gtk+-3.0
#include <gtk/gtk.h>
*/
import "C"

import "github.com/webview/webview_go"

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
