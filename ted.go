package ted

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
)

// loadSection gets the uint16 length and reads that minus 2 bytes
// into the returned byte slice. It returns an error when reads
// fail or are incomplete
func loadSection(r io.Reader) (data []byte, err error) {
	var (
		length    uint16
		bytesRead int
	)
	err = binary.Read(r, binary.LittleEndian, &length)
	if err != nil {
		return
	}
	data = make([]byte, int(length)-2)
	bytesRead, err = r.Read(data)
	if err == nil {
		return
	}
	if bytesRead != len(data) {
		err = errors.New("short read")
	}
	return
}
func parseSummary(r io.Reader) (sum summary, err error) {
	data, err := loadSection(r)
	if err != nil {
		return sum, err
	}
	dbuf := bytes.Buffer{}
	if n, err := dbuf.Write(data); n != len(data) || err != nil {
		return sum, errors.New("failed to write summary data to buffer")
	}
	fill := make([]byte, 32)
	if n, err := dbuf.Write(fill); n != len(fill) || err != nil {
		return sum, errors.New("failed to write summary data to buffer")
	}
	err = binary.Read(&dbuf, binary.LittleEndian, &sum)
	return
}

func parseLobbyChat(r io.Reader) (chat lobbyChat, err error) {
	data, err := loadSection(r)
	if err != nil {
		return chat, err
	}
	chatlog := data[4:]
	raw := bytes.Split(chatlog, []byte{0x0d})
	chat.Messages = make([]string, len(raw)-1)
	for i := range chat.Messages {
		chat.Messages[i] = string(raw[i])
	}
	return
}

func parseAddressBlock(r io.Reader) (ab addressBlock, err error) {
	data, err := loadSection(r)
	if err != nil {
		return ab, err
	}
	addressData := simpleCrypt(data[4:])
	ip := bytes.Split(addressData[0x50:], []byte{0x0})
	ab.IP = string(ip[0])
	return
}
func parsePlayer(r io.Reader) (pb playerBlock, err error) {
	data, err := loadSection(r)
	if err != nil {
		return pb, err
	}
	dbuf := bytes.Buffer{}
	if n, err := dbuf.Write(data); n != len(data) || err != nil {
		return pb, errors.New("failed to write player data to buffer")
	}
	fill := make([]byte, 32)
	if n, err := dbuf.Write(fill); n != len(fill) || err != nil {
		return pb, errors.New("failed to write player data to buffer")
	}
	err = binary.Read(&dbuf, binary.LittleEndian, &pb)
	return
}

func simpleCrypt(in []byte) []byte {
	out := make([]byte, len(in))
	for i := range in {
		out[i] = in[i]
	}
	for i := range out {
		out[i] = out[i] ^ 42
	}
	return out
}