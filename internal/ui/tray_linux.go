//go:build linux

package ui

/*
#cgo pkg-config: gtk+-3.0
#include <gtk/gtk.h>
#include <stdint.h>

typedef enum { TRAY_SHOW, TRAY_HIDE, TRAY_VERSION, TRAY_CLOSE } tray_action_t;

static GtkStatusIcon *tray_icon = NULL;
static GtkWidget *tray_menu = NULL;

extern void trayActionCallback(int);

static void tray_menu_item_cb(GtkMenuItem *item, gpointer data) {
    trayActionCallback((int)(intptr_t)data);
}

static void tray_activate(GtkStatusIcon *status_icon, gpointer user_data) {
    trayActionCallback(TRAY_SHOW);
}

static void tray_popup(GtkStatusIcon *status_icon, guint button, guint activate_time, gpointer user_data) {
    if (!tray_menu) return;
    gtk_menu_popup_at_pointer(GTK_MENU(tray_menu), NULL);
}

static void tray_init() {
    if (tray_icon) return;
    tray_icon = gtk_status_icon_new_from_icon_name("utilities-terminal");
    gtk_status_icon_set_visible(tray_icon, TRUE);

    GtkWidget *show_item = gtk_menu_item_new_with_label("Show pTerminal");
    GtkWidget *hide_item = gtk_menu_item_new_with_label("Hide Window");
    GtkWidget *version_item = gtk_menu_item_new_with_label("About pTerminal");
    GtkWidget *close_item = gtk_menu_item_new_with_label("Exit pTerminal");

    g_signal_connect(show_item, "activate", G_CALLBACK(tray_menu_item_cb), GINT_TO_POINTER(TRAY_SHOW));
    g_signal_connect(hide_item, "activate", G_CALLBACK(tray_menu_item_cb), GINT_TO_POINTER(TRAY_HIDE));
    g_signal_connect(version_item, "activate", G_CALLBACK(tray_menu_item_cb), GINT_TO_POINTER(TRAY_VERSION));
    g_signal_connect(close_item, "activate", G_CALLBACK(tray_menu_item_cb), GINT_TO_POINTER(TRAY_CLOSE));

    tray_menu = gtk_menu_new();
    gtk_menu_shell_append(GTK_MENU_SHELL(tray_menu), show_item);
    gtk_menu_shell_append(GTK_MENU_SHELL(tray_menu), hide_item);
    gtk_menu_shell_append(GTK_MENU_SHELL(tray_menu), version_item);
    gtk_menu_shell_append(GTK_MENU_SHELL(tray_menu), gtk_separator_menu_item_new());
    gtk_menu_shell_append(GTK_MENU_SHELL(tray_menu), close_item);
    gtk_widget_show_all(tray_menu);

    g_signal_connect(tray_icon, "activate", G_CALLBACK(tray_activate), NULL);
    g_signal_connect(tray_icon, "popup-menu", G_CALLBACK(tray_popup), NULL);
}

static void tray_cleanup() {
    if (tray_icon) {
        gtk_status_icon_set_visible(tray_icon, FALSE);
        g_object_unref(tray_icon);
        tray_icon = NULL;
    }
    if (tray_menu) {
        gtk_widget_destroy(tray_menu);
        tray_menu = NULL;
    }
}

static gboolean tray_confirm_close() {
    GtkWidget *dialog = gtk_message_dialog_new(NULL, GTK_DIALOG_MODAL, GTK_MESSAGE_QUESTION,
                                               GTK_BUTTONS_YES_NO, "Close pTerminal?");
    gint response = gtk_dialog_run(GTK_DIALOG(dialog));
    gtk_widget_destroy(dialog);
    return response == GTK_RESPONSE_YES;
}

static void tray_show_version(const char *version) {
    GtkWidget *dialog = gtk_message_dialog_new(NULL, GTK_DIALOG_MODAL, GTK_MESSAGE_INFO,
                                               GTK_BUTTONS_OK, "%s", version);
    gtk_window_set_title(GTK_WINDOW(dialog), "About pTerminal");
    gtk_dialog_run(GTK_DIALOG(dialog));
    gtk_widget_destroy(dialog);
}
*/
import "C"

import (
	"github.com/ankouros/pterminal/internal/buildinfo"
	"unsafe"
)

var trayWindow *Window

const (
	trayActionShow    = 0
	trayActionHide    = 1
	trayActionVersion = 2
	trayActionClose   = 3
)

func trayInit(w *Window) {
	if w == nil {
		return
	}
	trayWindow = w
	C.tray_init()
}

func trayCleanup() {
	C.tray_cleanup()
}

func confirmCloseDialog() bool {
	return C.tray_confirm_close() == C.TRUE
}

//export trayActionCallback
func trayActionCallback(action C.int) {
	if trayWindow == nil {
		return
	}
	switch int(action) {
	case trayActionShow:
		trayWindow.showFromTray()
	case trayActionHide:
		trayWindow.hideFromTray()
	case trayActionVersion:
		trayWindow.showVersion()
	case trayActionClose:
		trayWindow.closeFromTray()
	}
}

func (w *Window) showVersion() {
	if w == nil {
		return
	}
	info := buildinfo.String()
	cstr := C.CString(info)
	C.tray_show_version(cstr)
	C.free(unsafe.Pointer(cstr))
}
