package ted

type summary struct {
	Magic      [8]byte
	Version    [2]byte
	NumPlayers uint8
	MaxUnits   uint16
	MapName    [32]byte
}

// func (s *summary) sectorType() string {
// 	return "Summary"
// }

type lobbyChat struct {
	Messages []string
}

type addressBlock struct {
	IP string
}

// func (s *lobbyChat) sectorType() string {
// 	return "LobbyChat"
// }

// type logSector interface {
// 	sectorType() string
// }
