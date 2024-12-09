package ipmi

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"time"
	"unsafe"
)

// IPMI DCMI related constants.
const (
	IPMI_LAN             = 0x1 //nolint:stylecheck
	IPMI_LANP_IP_ADDR    = 0x2 //nolint:stylecheck
	IPMI_NETFN_TRANSPORT = 0xC //nolint:stylecheck
)

// LanIP returns the IP address of BMC.
func (i *IPMIClient) LanIP(timeout time.Duration) (*string, error) {
	// Request payload
	msgData := [4]uint8{IPMI_LAN, 0x3, 0x0, 0x0}

	// IPMI Request
	req := ipmiReq{
		Addr:    uintptr(unsafe.Pointer(&i.BMCAddr)),
		AddrLen: uint(unsafe.Sizeof(i.BMCAddr)),
		Msgid:   1,
		Msg: ipmiMsg{
			Data:    uintptr(unsafe.Pointer(&msgData[0])),
			DataLen: 4,
			Netfn:   IPMI_NETFN_TRANSPORT,
			Cmd:     IPMI_LANP_IP_ADDR,
		},
	}

	// Do request and read response
	resp, err := i.Do(&req, timeout)
	if err != nil {
		i.Logger.Error("Failed to make IPMI request", "err", err)

		return nil, fmt.Errorf("failed to make IPMI request: %w", err)
	}

	// Check completion code
	var completionCode uint16
	if err := binary.Read(bytes.NewReader(resp.Data[0:1]), binary.BigEndian, &completionCode); err == nil && completionCode != 0 {
		return nil, errors.New("received non zero completion code for IPMI LAN IP response")
	}

	// Get LAN IP
	ip := fmt.Sprintf("%d.%d.%d.%d", resp.Data[2], resp.Data[3], resp.Data[4], resp.Data[5])

	return &ip, nil
}
