//go:build linux

package tapeio

import (
	"syscall"
	"unsafe"
)

// See https://github.com/benmcclelland/mtio
const (
	mtioCpos = 0x80086d03 // Get tape position
	mtioCtop = 0x40086d01 // Do magnetic tape operation

	mtFsf  = 1  // Forward space over FileMark, position at first record of next file
	mtOffl = 7  // Rewind and put the drive offline (eject?)
	mtEom  = 12 // Goto end of recorded media (for appending files)
	mtSeek = 22 // Seek to block
)

// position is struct for MTIOCPOS
type position struct {
	blkNo int64 // Current block number
}

// operation is struct for MTIOCTOP
type operation struct {
	op    int16 // Operation ID
	pad   int16 // Padding to match C structures
	count int32 // Operation count
}

func (t *Tape) GetCurrentRecordFromTape(fd uintptr) (int64, error) {
	pos := &position{}
	if _, _, err := syscall.Syscall(
		syscall.SYS_IOCTL,
		fd,
		mtioCpos,
		uintptr(unsafe.Pointer(pos)),
	); err != 0 {
		return 0, err
	}

	return pos.blkNo, nil
}

func (t *Tape) GoToEndOfTape(fd uintptr) error {
	if _, _, err := syscall.Syscall(
		syscall.SYS_IOCTL,
		fd,
		mtioCtop,
		uintptr(unsafe.Pointer(
			&operation{
				op: mtEom,
			},
		)),
	); err != 0 {
		return err
	}

	return nil
}

func (t *Tape) GoToNextFileOnTape(fd uintptr) error {
	if _, _, err := syscall.Syscall(
		syscall.SYS_IOCTL,
		fd,
		mtioCtop,
		uintptr(unsafe.Pointer(
			&operation{
				op:    mtFsf,
				count: 1,
			},
		)),
	); err != 0 {
		return err
	}

	return nil
}

func (t *Tape) EjectTape(fd uintptr) error {
	if _, _, err := syscall.Syscall(
		syscall.SYS_IOCTL,
		fd,
		mtioCtop,
		uintptr(unsafe.Pointer(
			&operation{
				op: mtOffl,
			},
		)),
	); err != 0 {
		return err
	}

	return nil
}

func (t *Tape) SeekToRecordOnTape(fd uintptr, record int32) error {
	if _, _, err := syscall.Syscall(
		syscall.SYS_IOCTL,
		fd,
		mtioCtop,
		uintptr(unsafe.Pointer(
			&operation{
				op:    mtSeek,
				count: record,
			},
		)),
	); err != 0 {
		return err
	}

	return nil
}
