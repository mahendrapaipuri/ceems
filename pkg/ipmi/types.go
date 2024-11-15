package ipmi

type Msg struct {
	Netfn     uint8
	Lun       uint8
	Cmd       uint8
	TargetCmd uint8
	DataLen   uint16
	Data      uintptr
}

type ipmiRs struct {
	Ccode   uint8
	Data    [1024]uint8
	DataLen int32
}

type ipmiAddr struct {
	AddrType int
	Channel  int16
	Data     [32]byte
}

type ipmiMsg struct {
	Netfn   uint8
	Cmd     uint8
	DataLen uint16
	Data    uintptr
}

type ipmiRecv struct {
	RecvType int
	Addr     uintptr
	AddrLen  uint
	Msgid    int
	Msg      ipmiMsg
}

type ipmiSystemInterfaceAddr struct {
	AddrType uint32
	Channel  uint8
	Lun      uint8
}

type ipmiReq struct {
	Addr    uintptr
	AddrLen uint
	Msgid   int
	Msg     ipmiMsg
}
