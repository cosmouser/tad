package tad

type sectorType int32

const (
	commentsType sectorType = iota + 1
	lobbyChatType
	versionNumberType
	dateStringType
	recFromType
	playerAddrType
)

type summary struct {
	Magic      [8]byte
	Version    [2]byte
	NumPlayers uint8
	MaxUnits   uint16
	MapName    [64]byte
}

type extraSector struct {
	sectorType // int32
	data       []byte
}
type statusMsg struct {
	Number byte
	Data   []byte
}

type lobbyChat struct {
	Messages []string
}

type addressBlock struct {
	IP string
}

type playerBlock struct {
	Color  byte
	Side   byte // 0=arm,1=core,2=watch
	Number byte
	Name   [64]byte
}

type packetRec struct {
	Time   uint16
	Sender byte
	Data   []byte
}

type savePlayers struct {
	TimeToDie [10]int
	Killed    [10]bool
}
type identRec struct {
	Fill1   [139]byte
	Width   uint16
	Height  uint16
	Fill3   byte
	Player1 int32
	Data2   [7]byte
	Clicked byte
	Fill2   [9]byte
	Data5   uint16
	HiVer   byte
	LoVer   byte
	Data3   [17]byte
	Player2 int32
	Data4   byte
}

// DemoPlayer is the in memory representation of a player
type DemoPlayer struct {
	Color  byte
	Side   byte
	Number byte
	Name   string
	Status string
	orgpid int32
}
type unitSyncRecord struct {
	ID    uint32
	CRC   uint32
	InUse bool
	Limit uint16
}

type unitSync02 struct {
	Marker byte
	Sub    byte
	_      [4]byte
	ID     uint32
	CRC    uint32
}
type unitSync03 struct {
	Marker byte
	Sub    byte
	_      [4]byte
	ID     uint32
	Status uint16
	Limit  uint16
}
type saveHealth struct {
	MaxUnits int32
	Health   [5001]int32
}
