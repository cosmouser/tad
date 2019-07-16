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

// Game holds the state of the replay parser
type Game struct {
	MapName    string
	Players    []DemoPlayer
	LobbyChat  []string
	Version    string
	RecFrom    string
	RecDate    string
	MaxUnits   int
	TimeToDie  [10]int
	TotalMoves int
	Unitsum    string
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

// PacketRec is a subpacket of the move referred to in IdemToken
type PacketRec struct {
	Time      uint16 // time since last packet in milliseconds
	Sender    byte
	IdemToken string // idempotency token, arbitrary uuid
	Data      []byte
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
	IP     string
	Cheats bool
	TDPID  int32
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

// PlaybackFrame has the locations of each unit in the game at a specific point in time
// It is used by DrawGif
type PlaybackFrame struct {
	Number int
	Time   int
	Units  map[uint16]*TAUnit
}

// TAUnit is a unit in the game
type TAUnit struct {
	Owner    int
	NetID    uint16
	Finished bool
	ID       string // uuid per unit per game
	Pos      point
	NextPos  point
	Class    unitClass
}
type point struct {
	X    int
	Y    int
	ID   string
	Time int
}

// FinalScore holds score info for gamedata
type FinalScore struct {
	Status  int     `json:"status"`
	Won     int     `json:"won"`
	Lost    int     `json:"lost"`
	Player  string  `json:"player"`
	Kills   int     `json:"kills"`
	Losses  int     `json:"losses"`
	TotalE  float64 `json:"energyProduced"`
	ExcessE float64 `json:"excessEnergy"`
	TotalM  float64 `json:"metalProduced"`
	ExcessM float64 `json:"excessMetal"`
}

// SPLite is a smaller version of a score packet. It only contains m/e per second.
// It also has a time value for easy plotting.
type SPLite struct {
	Kills        int
	Losses       int
	Metal        float64
	Energy       float64
	TotalE       float64
	TotalM       float64
	ExcessE      float64
	ExcessM      float64
	Milliseconds int
}
type scoreError struct {
	player       string
	playerNumber int
}

func (s *scoreError) Error() string {
	return "detected foul play"
}
