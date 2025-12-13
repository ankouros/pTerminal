//go:build linux

package ui

/*
#cgo pkg-config: gtk+-3.0
#include <stdlib.h>
#include <gtk/gtk.h>

static void pterminal_set_window_icon_from_file(void* win, const char* path) {
  if (win == NULL || path == NULL) return;
  GtkWindow* w = (GtkWindow*)win;
  GError* err = NULL;
  gtk_window_set_icon_from_file(w, path, &err);
  if (err != NULL) {
    g_error_free(err);
  }
}
*/
import "C"

import (
	"os"
	"path/filepath"
	"unsafe"
)

func (w *Window) setNativeIcon() {
	b, err := assets.ReadFile("assets/logo.svg")
	if err != nil || len(b) == 0 {
		return
	}

	dir := os.TempDir()
	p := filepath.Join(dir, "pterminal-icon.svg")
	_ = os.WriteFile(p, b, 0o600)

	cs := C.CString(p)
	defer C.free(unsafe.Pointer(cs))
	C.pterminal_set_window_icon_from_file(w.wv.Window(), cs)
}
