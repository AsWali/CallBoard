package main

import (
	"os"

	"golang.org/x/sys/windows"
)

func enableVT() func() {
	h := windows.Handle(os.Stdout.Fd())
	var mode uint32
	if windows.GetConsoleMode(h, &mode) != nil {
		return func() {}
	}
	windows.SetConsoleMode(h, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING)
	return func() { windows.SetConsoleMode(h, mode) }
}
