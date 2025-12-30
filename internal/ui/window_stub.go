//go:build !linux

package ui

import "github.com/webview/webview_go"

func hideGtkWindow(w webview.WebView)             {}
func showGtkWindow(w webview.WebView)             {}
func installWindowCloseHandler(w webview.WebView) {}
func centerGtkWindow(w webview.WebView)           {}
