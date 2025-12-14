//go:build linux

package ui

/*
#cgo pkg-config: gtk+-3.0
#include <stdlib.h>
#include <gtk/gtk.h>

static char* pterminal_pick_executable(void* parent, const char* title) {
  GtkWindow* w = (GtkWindow*)parent;
  GtkWidget* dialog = gtk_file_chooser_dialog_new(
    title,
    w,
    GTK_FILE_CHOOSER_ACTION_OPEN,
    "_Cancel", GTK_RESPONSE_CANCEL,
    "_Open", GTK_RESPONSE_ACCEPT,
    NULL
  );

  gtk_file_chooser_set_local_only(GTK_FILE_CHOOSER(dialog), TRUE);
  gtk_file_chooser_set_select_multiple(GTK_FILE_CHOOSER(dialog), FALSE);

  // Best-effort: suggest typical executable directories.
  gtk_file_chooser_set_current_folder(GTK_FILE_CHOOSER(dialog), "/home");

  char* filename = NULL;
  if (gtk_dialog_run(GTK_DIALOG(dialog)) == GTK_RESPONSE_ACCEPT) {
    filename = gtk_file_chooser_get_filename(GTK_FILE_CHOOSER(dialog));
  }

  gtk_widget_destroy(dialog);
  return filename; // must be freed by g_free()
}
*/
import "C"

import (
	"unsafe"
)

func (w *Window) pickIOShellExecutablePath() string {
	if w == nil || w.wv == nil {
		return ""
	}

	title := C.CString("Select IOshell executable")
	defer C.free(unsafe.Pointer(title))

	p := C.pterminal_pick_executable(w.wv.Window(), title)
	if p == nil {
		return ""
	}
	defer C.g_free(C.gpointer(p))
	return C.GoString(p)
}
