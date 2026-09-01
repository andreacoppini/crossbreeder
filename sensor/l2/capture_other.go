//go:build !linux

package l2

import (
	"context"
	"io"
	"time"
)

// Capture is Linux-only: it needs a packet socket. The rest of the sensor
// builds and its tests run on any platform, so this reports the limitation.
func Capture(context.Context, CaptureOptions, io.Writer) (CaptureStats, error) {
	return CaptureStats{}, errCaptureUnsupported
}

// Discover is Linux-only for the same reason.
func Discover(context.Context, string, time.Duration) ([]Neighbour, error) {
	return nil, errCaptureUnsupported
}
