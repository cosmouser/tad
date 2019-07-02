package tad

type sectorType int32

const (
	commentsType sectorType = iota+1
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
	MapName    [32]byte
}

type extraSector struct {
	sectorType
	data []byte
}

type lobbyChat struct {
	Messages []string
}

type addressBlock struct {
	IP string
}

type playerBlock struct {
	Color  byte
	Side   byte
	Number byte
	Name   [32]byte
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

type statusMessage struct {
}

// func (s *lobbyChat) sectorType() string {
// 	return "LobbyChat"
// }

// type logSector interface {
// 	sectorType() string
// }
