package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

const (
	rtIcon       = 3
	rtGroupIcon  = 14
	langNeutral  = 0x0409
	defaultGroup = 1
)

var (
	kernel                  = syscall.NewLazyDLL("kernel32.dll")
	procBeginUpdateResource = kernel.NewProc("BeginUpdateResourceW")
	procUpdateResource      = kernel.NewProc("UpdateResourceW")
	procEndUpdateResource   = kernel.NewProc("EndUpdateResourceW")
)

type iconDir struct {
	Reserved uint16
	Type     uint16
	Count    uint16
}

type iconDirEntry struct {
	Width       uint8
	Height      uint8
	ColorCount  uint8
	Reserved    uint8
	Planes      uint16
	BitCount    uint16
	BytesInRes  uint32
	ImageOffset uint32
}

type grpIconEntry struct {
	Width      uint8
	Height     uint8
	ColorCount uint8
	Reserved   uint8
	Planes     uint16
	BitCount   uint16
	BytesInRes uint32
	ID         uint16
}

func main() {
	exePath := flag.String("exe", "", "path to target exe")
	iconPath := flag.String("icon", "", "path to .ico file")
	flag.Parse()

	if *exePath == "" || *iconPath == "" {
		flag.Usage()
		os.Exit(1)
	}

	data, err := os.ReadFile(*iconPath)
	if err != nil {
		exitErr(fmt.Errorf("read icon: %w", err))
	}
	entries, err := parseICO(data)
	if err != nil {
		exitErr(fmt.Errorf("parse icon: %w", err))
	}
	handle, err := beginUpdate(*exePath)
	if err != nil {
		exitErr(err)
	}
	defer endUpdate(handle)

	for i, entry := range entries {
		if err := updateIcon(handle, uint16(i+1), entry.Data); err != nil {
			exitErr(err)
		}
	}
	group, err := buildGroup(entries)
	if err != nil {
		exitErr(err)
	}
	if err := updateGroup(handle, defaultGroup, group); err != nil {
		exitErr(err)
	}
}

type icoEntry struct {
	Meta iconDirEntry
	Data []byte
}

func parseICO(data []byte) ([]icoEntry, error) {
	r := bytes.NewReader(data)
	var hdr iconDir
	if err := binary.Read(r, binary.LittleEndian, &hdr); err != nil {
		return nil, err
	}
	if hdr.Type != 1 || hdr.Count == 0 {
		return nil, fmt.Errorf("invalid icon file")
	}
	entries := make([]icoEntry, hdr.Count)
	for i := uint16(0); i < hdr.Count; i++ {
		var e iconDirEntry
		if err := binary.Read(r, binary.LittleEndian, &e); err != nil {
			return nil, err
		}
		if int(e.ImageOffset+e.BytesInRes) > len(data) {
			return nil, fmt.Errorf("icon data out of range")
		}
		chunk := make([]byte, e.BytesInRes)
		copy(chunk, data[e.ImageOffset:e.ImageOffset+e.BytesInRes])
		entries[i] = icoEntry{Meta: e, Data: chunk}
	}
	return entries, nil
}

func beginUpdate(exe string) (syscall.Handle, error) {
	ptr, err := syscall.UTF16PtrFromString(exe)
	if err != nil {
		return 0, err
	}
	handle, _, callErr := procBeginUpdateResource.Call(uintptr(unsafe.Pointer(ptr)), uintptr(0))
	if handle == 0 {
		if callErr != nil && callErr != syscall.Errno(0) {
			return 0, callErr
		}
		return 0, fmt.Errorf("BeginUpdateResource failed")
	}
	return syscall.Handle(handle), nil
}

func updateIcon(handle syscall.Handle, id uint16, data []byte) error {
	if len(data) == 0 {
		return fmt.Errorf("empty icon data")
	}
	return updateResource(handle, rtIcon, id, data)
}

func buildGroup(entries []icoEntry) ([]byte, error) {
	buf := &bytes.Buffer{}
	hdr := iconDir{Type: 1, Count: uint16(len(entries))}
	if err := binary.Write(buf, binary.LittleEndian, hdr); err != nil {
		return nil, err
	}
	for i, entry := range entries {
		meta := entry.Meta
		grp := grpIconEntry{
			Width:      meta.Width,
			Height:     meta.Height,
			ColorCount: meta.ColorCount,
			Planes:     meta.Planes,
			BitCount:   meta.BitCount,
			BytesInRes: uint32(len(entry.Data)),
			ID:         uint16(i + 1),
		}
		if err := binary.Write(buf, binary.LittleEndian, grp); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

func updateGroup(handle syscall.Handle, id uint16, data []byte) error {
	return updateResource(handle, rtGroupIcon, id, data)
}

func updateResource(handle syscall.Handle, resType uint16, id uint16, data []byte) error {
	if len(data) == 0 {
		return fmt.Errorf("resource data empty")
	}
	ret, _, err := procUpdateResource.Call(
		uintptr(handle),
		uintptr(resType),
		uintptr(id),
		uintptr(langNeutral),
		uintptr(unsafe.Pointer(&data[0])),
		uintptr(len(data)),
	)
	if ret == 0 {
		if err != nil && err != syscall.Errno(0) {
			return err
		}
		return fmt.Errorf("UpdateResource failed")
	}
	return nil
}

func endUpdate(handle syscall.Handle) {
	ret, _, err := procEndUpdateResource.Call(uintptr(handle), uintptr(0))
	if ret == 0 && err != nil && err != syscall.Errno(0) {
		fmt.Fprintf(os.Stderr, "EndUpdateResource: %v\n", err)
	}
}

func exitErr(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
