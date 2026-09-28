//go:build !windows

package main

func enableVT() func() { return func() {} }
