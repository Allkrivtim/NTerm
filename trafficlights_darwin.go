//go:build darwin

package main

/*
#cgo CFLAGS: -fblocks
#cgo LDFLAGS: -framework Cocoa

void NTermInstallTrafficLightAlignment(double centerFromTop);
*/
import "C"

func installTrafficLightAlignment() {
	// The original 40 px tab strip has a vertical centre at 20 px.
	C.NTermInstallTrafficLightAlignment(20)
}
