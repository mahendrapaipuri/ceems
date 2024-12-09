// Package ipmi implements in-band communication with BMC using IPMI commands
// using `/dev/ipmi*` device.
package ipmi

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// IPMI related constants.
const (
	IPMICTL_SET_GETS_EVENTS_CMD     = 0x80046910 //nolint:stylecheck
	IPMICTL_SEND_COMMAND            = 0x8028690d //nolint:stylecheck
	IPMICTL_RECEIVE_MSG_TRUNC       = 0xc030690b //nolint:stylecheck
	IPMI_SYSTEM_INTERFACE_ADDR_TYPE = 0xC        //nolint:stylecheck
	IPMI_BMC_CHANNEL                = 0xF        //nolint:stylecheck
)

type IPMIClient struct {
	Logger  *slog.Logger
	DevFile *os.File
	BMCAddr ipmiSystemInterfaceAddr
}

// NewIPMIClient returns a new instance of IPMIClient struct.
func NewIPMIClient(devNum int, logger *slog.Logger) (*IPMIClient, error) {
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

	return &IPMIClient{
		Logger:  logger,
		DevFile: devFile,
		BMCAddr: ipmiSystemInterfaceAddr{
			AddrType: IPMI_SYSTEM_INTERFACE_ADDR_TYPE,
			Channel:  IPMI_BMC_CHANNEL,
			Lun:      0x0,
		},
	}, nil
}

// Do sends IPMI request and returns the response.
func (i *IPMIClient) Do(req *ipmiReq, timeout time.Duration) (*ipmiResp, error) {
	// Device file descriptor
	fd := i.DevFile.Fd()

	// Send request
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, IPMICTL_SEND_COMMAND, uintptr(unsafe.Pointer(req))); errno != 0 {
		i.Logger.Error("Failed to send IPMI request", "err", errno)

		return nil, fmt.Errorf("failed to send IPMI request: %w", errno)
	}

	var activeFdSet unix.FdSet

	serverFD := int(fd) //nolint: gosec

	FDZero(&activeFdSet)
	FDSet(serverFD, &activeFdSet)

	resp := ipmiResp{}
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
	start := time.Now()

	for {
		_, err := unix.Select(serverFD+1, &activeFdSet, nil, nil, nil)
		if err != nil {
			i.Logger.Error("Failed to receive response from IPMI device interface", "err", err)

			return nil, fmt.Errorf("failed to receive response from IPMI device interface: %w", err)
		}

		if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, IPMICTL_RECEIVE_MSG_TRUNC, uintptr(unsafe.Pointer(&recv))); errno != 0 {
			i.Logger.Error("Failed to read response from IPMI device interface", "err", errno)

			return nil, fmt.Errorf("failed to read response from IPMI device interface: %w", errno)
		}

		// If Msgids match between response and request break
		if req.Msgid == recv.Msgid {
			i.Logger.Debug("IPMI response received from device interface")

			break
		}

		// Check if we reached timeout
		if time.Since(start) > timeout {
			i.Logger.Error("IPMI response timed out")

			return nil, errors.New("timeout exceeded waiting for response from IPMI device interface")
		}
	}

	// Read response data
	resp.DataLen = int32(recv.Msg.DataLen)
	i.Logger.Debug("IPMI response data", "data", resp.Data[0:resp.DataLen])

	return &resp, nil
}

// Close IPMI device file.
func (i *IPMIClient) Close() error {
	return i.DevFile.Close()
}
