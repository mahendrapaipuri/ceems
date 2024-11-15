// Package ipmi implements in-band `dcmi power reading` command to get power
// reading from BMC using `/dev/ipmi*` device.
package ipmi

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// IPMI DCMI constants.
const (
	IPMICTL_SET_GETS_EVENTS_CMD     = 0x80046910 //nolint: stylecheck
	IPMICTL_SEND_COMMAND            = 0x8028690d //nolint: stylecheck
	IPMICTL_RECEIVE_MSG_TRUNC       = 0xc030690b //nolint: stylecheck
	IPMI_DCMI                       = 0xDC       //nolint: stylecheck
	IPMI_DCMI_GETRED                = 0x2        //nolint: stylecheck
	IPMI_NETFN_DCGRP                = 0x2C       //nolint: stylecheck
	IPMI_SYSTEM_INTERFACE_ADDR_TYPE = 0xC        //nolint: stylecheck
	IPMI_BMC_CHANNEL                = 0xF        //nolint: stylecheck
	IPMI_DCMI_ACTIVATED             = 0x40       //nolint: stylecheck
)

type IPMIDCMI struct {
	Logger  *slog.Logger
	DevFile *os.File
}

type PowerReading struct {
	Minimum, Maximum, Average, Current uint16
	Activated                          bool
}

// NewIPMIDCMI returns a new instance of IPMIDCMI struct.
func NewIPMIDCMI(devNum int, logger *slog.Logger) (*IPMIDCMI, error) {
	if devNum < 0 {
		return nil, errors.New("device number for IPMI must be greater than zero")
	}

	// List of devices to verify in the order of preference
	ipmiDevs := []string{"/dev/ipmi%d", "/dev/ipmi/%d", "/dev/ipmidev/%d"}

	// Attempt to open device file
	var devFile *os.File

	for _, d := range ipmiDevs {
		if f, err := os.Open(fmt.Sprintf(d, devNum)); err == nil {
			logger.Debug("IPMI device found", "device", fmt.Sprintf(d, devNum))
			devFile = f

			break
		}
	}

	// If no device is found, return error
	if devFile == nil {
		return nil, errors.New("no IPMI device found on the host")
	}

	// Setup event receiver
	var recvEvents int = 1
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, devFile.Fd(), IPMICTL_SET_GETS_EVENTS_CMD, uintptr(unsafe.Pointer(&recvEvents))); errno != 0 {
		return nil, fmt.Errorf("failed to enable IPMI event receiver: %w", errno)
	}

	return &IPMIDCMI{
		Logger:  logger,
		DevFile: devFile,
	}, nil
}

// PowerReading returns the current IPMI DCMI power reading.
func (i *IPMIDCMI) PowerReading() (*PowerReading, error) {
	// Request payload
	msgData := [4]uint8{IPMI_DCMI, 0x1, 0x0, 0x0}

	// BMC Addresses
	bmcAddr := ipmiSystemInterfaceAddr{
		AddrType: IPMI_SYSTEM_INTERFACE_ADDR_TYPE,
		Channel:  IPMI_BMC_CHANNEL,
		Lun:      0x0,
	}

	// IPMI Request
	req := ipmiReq{
		Addr:    uintptr(unsafe.Pointer(&bmcAddr)),
		AddrLen: uint(unsafe.Sizeof(bmcAddr)),
		Msgid:   1,
		Msg: ipmiMsg{
			Data:    uintptr(unsafe.Pointer(&msgData[0])),
			DataLen: 4,
			Netfn:   IPMI_NETFN_DCGRP,
			Cmd:     IPMI_DCMI_GETRED,
		},
	}

	// Device file descriptor
	fd := i.DevFile.Fd()

	// Send request
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, IPMICTL_SEND_COMMAND, uintptr(unsafe.Pointer(&req))); errno != 0 {
		i.Logger.Error("Failed to send IPMI request", "err", errno)

		return nil, fmt.Errorf("failed to send IPMI request: %w", errno)
	}

	var activeFdSet unix.FdSet

	serverFD := int(fd) //nolint: gosec

	FDZero(&activeFdSet)
	FDSet(serverFD, &activeFdSet)

	resp := ipmiRs{}
	addr := ipmiAddr{}
	recv := ipmiRecv{
		Addr:    uintptr(unsafe.Pointer(&addr)),
		AddrLen: uint(unsafe.Sizeof(addr)),
		Msg: ipmiMsg{
			Data:    uintptr(unsafe.Pointer(&resp.Data)),
			DataLen: uint16(unsafe.Sizeof(resp.Data)),
		},
	}

	// Read the response back
	for {
		_, err := unix.Select(serverFD+1, &activeFdSet, nil, nil, nil)
		if err != nil {
			i.Logger.Error("Failed to receive IPMI response", "err", err)

			return nil, fmt.Errorf("failed to receive IPMI response: %w", err)
		}

		if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, IPMICTL_RECEIVE_MSG_TRUNC, uintptr(unsafe.Pointer(&recv))); errno != 0 {
			i.Logger.Error("Failed to read IPMI response", "err", errno)

			return nil, fmt.Errorf("failed to read IPMI response: %w", errno)
		}

		// If Msgids match between response and request break
		if req.Msgid == recv.Msgid {
			i.Logger.Debug("IPMI response received")

			break
		}
	}

	// Read response data
	resp.DataLen = int32(recv.Msg.DataLen)
	i.Logger.Debug("IPMI response data", "data", resp.Data[0:resp.DataLen])

	// Check completion code
	var completionCode uint16
	if err := binary.Read(bytes.NewReader(resp.Data[0:1]), binary.BigEndian, &completionCode); err == nil && completionCode != 0 {
		return nil, errors.New("received non zero completion code for IPMI power readings response")
	}

	// Get readings
	return &PowerReading{
		Current:   binary.LittleEndian.Uint16(resp.Data[2:4]),
		Minimum:   binary.LittleEndian.Uint16(resp.Data[4:6]),
		Maximum:   binary.LittleEndian.Uint16(resp.Data[6:8]),
		Average:   binary.LittleEndian.Uint16(resp.Data[8:10]),
		Activated: resp.Data[18] == IPMI_DCMI_ACTIVATED,
	}, nil
}

// Close IPMI device file.
func (i *IPMIDCMI) Close() error {
	return i.DevFile.Close()
}
