//go:build !linux

package tapeio

import (
	"github.com/pojntfx/webrtcfd/pkg/config"
)

func (t *Tape) GetCurrentRecordFromTape(fd uintptr) (int64, error) {
	return -1, config.ErrTapeDrivesUnsupported
}

func (t *Tape) GoToEndOfTape(fd uintptr) error {
	return config.ErrTapeDrivesUnsupported
}

func (t *Tape) GoToNextFileOnTape(fd uintptr) error {
	return config.ErrTapeDrivesUnsupported
}

func (t *Tape) EjectTape(fd uintptr) error {
	return config.ErrTapeDrivesUnsupported
}

func (t *Tape) SeekToRecordOnTape(fd uintptr, record int32) error {
	return config.ErrTapeDrivesUnsupported
}
