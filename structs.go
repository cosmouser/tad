package ted

type summary struct {
	Magic      [8]byte
	Version    [2]byte
	NumPlayers uint8
	MaxUnits   uint16
	MapName    [32]byte
}

type lobbyMessage struct {
	Sender string
	Body   string
}

type logSector interface {
	values() map[string]interface{}
	sectorType() string
}
