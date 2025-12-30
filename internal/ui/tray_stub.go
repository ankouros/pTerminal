//go:build !linux

package ui

func trayInit(w *Window)       {}
func trayCleanup()             {}
func confirmCloseDialog() bool { return true }
