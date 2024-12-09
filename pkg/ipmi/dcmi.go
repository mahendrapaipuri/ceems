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
	IPMI_DCMI           = 0xDC //nolint:stylecheck
	IPMI_DCMI_GETRED    = 0x2  //nolint:stylecheck
	IPMI_NETFN_DCGRP    = 0x2C //nolint:stylecheck
	IPMI_DCMI_ACTIVATED = 0x40 //nolint:stylecheck
)

type PowerReading struct {
	Minimum, Maximum, Average, Current uint16
	Activated                          bool
}

// PowerReading returns the current IPMI DCMI power reading.
func (i *IPMIClient) PowerReading(timeout time.Duration) (*PowerReading, error) {
	// Request payload
	msgData := [4]uint8{IPMI_DCMI, 0x1, 0x0, 0x0}

	// IPMI Request
	req := ipmiReq{
		Addr:    uintptr(unsafe.Pointer(&i.BMCAddr)),
		AddrLen: uint(unsafe.Sizeof(i.BMCAddr)),
		Msgid:   1,
		Msg: ipmiMsg{
			Data:    uintptr(unsafe.Pointer(&msgData[0])),
			DataLen: 4,
			Netfn:   IPMI_NETFN_DCGRP,
			Cmd:     IPMI_DCMI_GETRED,
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
