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
// game holds the state of the replay parser
type game struct {
	MapName string
	Players []DemoPlayer
	LobbyChat []string
	Version string
	RecFrom string
	RecDate string
	MaxUnits int
	TimeToDie [10]int
	TotalMoves int
	Unitsum string
}

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

type playerBlock struct {
	Color  byte
	Side   byte // 0=arm,1=core,2=watch
	Number byte
	Name   [64]byte
}

type packetRec struct {
	Time   uint16 // time since last packet in milliseconds
	Sender byte
	IdemToken   string // idempotency token, arbitrary uuid
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
	Data2   [7]byte // Data2[2] is player color
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
	IP string
	Cheats bool
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
type playbackFrame struct {
	Number int
	Time int
	Units map[uint16]*taUnit
}
type taUnit struct {
	Owner int
	NetID uint16
	Finished bool
	Pos point
	NextPos point
	ID string
}
type point struct {
	X int
	Y int
	ID string
	Time int
}
