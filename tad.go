package tad

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	log "github.com/sirupsen/logrus"
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
func parseExtra(secData []byte) (extra extraSector, err error) {
	var n int
	secReader := bytes.NewReader(secData)
	err = binary.Read(secReader, binary.LittleEndian, &extra.sectorType)
	if err != nil {
		return
	}
	remBytes := len(secData) - 4
	extra.data = make([]byte, remBytes)
	n, err = secReader.Read(extra.data)
	if n != remBytes {
		return extra, errors.New("parseExtra made short read")
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
	fill := make([]byte, 64)
	if n, err := dbuf.Write(fill); n != len(fill) || err != nil {
		return sum, errors.New("failed to write summary data to buffer")
	}
	err = binary.Read(&dbuf, binary.LittleEndian, &sum)
	return
}

func parseLobbyChat(extra extraSector) (chat lobbyChat, err error) {
	raw := bytes.Split(extra.data, []byte{0x0d})
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
	fill := make([]byte, 64)
	if n, err := dbuf.Write(fill); n != len(fill) || err != nil {
		return pb, errors.New("failed to write player data to buffer")
	}
	err = binary.Read(&dbuf, binary.LittleEndian, &pb)
	return
}
func parseStatMsg(r io.Reader) (sm statusMsg, err error) {
	data, err := loadSection(r)
	if err != nil {
		return sm, err
	}
	sr := bytes.NewReader(data)
	b, err := sr.ReadByte()
	if err != nil {
		return sm, err
	}
	sm.Number = b
	dataLen := len(data) - 1
	sm.Data = make([]byte, dataLen)
	n, err := sr.Read(sm.Data)
	if err != nil || n != dataLen {
		return sm, errors.New("parseStatMsg failed read")
	}
	return
}

func parseUnitSyncData(r io.Reader) (units map[uint32]*unitSyncRecord, err error) {
	var buf [14]byte
	var n int
	br := bytes.NewReader(buf[:])
	data, err := loadSection(r)
	if err != nil {
		return units, err
	}
	usr := bytes.NewReader(data)
	units = make(map[uint32]*unitSyncRecord)
	for err != io.EOF {
		n, err = usr.Read(buf[:])
		if n == 14 {
			if buf[1] == 0x02 {
				br.Reset(buf[:])
				tmp := unitSync02{}
				err = binary.Read(br, binary.LittleEndian, &tmp)
				if _, ok := units[tmp.ID]; !ok {
					units[tmp.ID] = &unitSyncRecord{}
				}
				units[tmp.ID].ID = tmp.ID
				units[tmp.ID].CRC = tmp.CRC
			}
			if buf[1] == 0x03 {
				br.Reset(buf[:])
				tmp := unitSync03{}
				err = binary.Read(br, binary.LittleEndian, &tmp)
				if _, ok := units[tmp.ID]; !ok {
					units[tmp.ID] = &unitSyncRecord{}
				}
				units[tmp.ID].ID = tmp.ID
				units[tmp.ID].Limit = tmp.Limit
				if tmp.Status != 1 {
					units[tmp.ID].InUse = true
				}
			}
		}
	}
	err = nil
	return
}

func loadMove(r io.Reader) (pr packetRec, err error) {
	dat, err := loadSection(r)
	if err != nil {
		return pr, err
	}
	datr := bytes.NewReader(dat)
	err = binary.Read(datr, binary.LittleEndian, &pr.Time)
	if err != nil {
		return pr, err
	}
	sender, err := datr.ReadByte()
	if err != nil {
		return pr, err
	}
	pr.Sender = sender
	datLen := len(dat) - 3
	pr.Data = make([]byte, datLen)
	if n, err := datr.Read(pr.Data); n != datLen || err != nil {
		return pr, errors.New("packet read failed")
	}
	return
}

func createIdent(fdata []byte) (idn identRec, err error) {
	ir := bytes.NewReader(fdata[8:])
	err = binary.Read(ir, binary.LittleEndian, &idn)
	return
}

func createPacket(raw []byte) (out []byte, err error) {
	tmp := []byte{}
	tmp, err = decryptPacket(raw)
	if tmp[0] == 0x04 {
		out, err = decompressLZ77(tmp, 3)
		return
	}
	return tmp, nil
}
func decompressLZ77(compressed []byte, prefixLen int) (decompressed []byte, err error) {
	var window [4096]byte
	var windowPos = 1
	var writeBuf bytes.Buffer
	if compressed[0] != 0x04 {
		return compressed, nil
	}
	if err := writeBuf.WriteByte(0x03); err != nil {
		return nil, err
	}
	if prefixLen > 1 {
		if n, err := writeBuf.Write(compressed[1:prefixLen]); n != prefixLen-1 || err != nil {
			return nil, err
		}
	}
	reader := bytes.NewReader(compressed[prefixLen:])
	for {
		tag, err := reader.ReadByte()
		if err != nil {
			return nil, err
		}
		for i := 0; i < 8; i++ {
			if (tag & 1) == 0 {
				value, err := reader.ReadByte()
				if err != nil {
					return nil, err
				}
				err = writeBuf.WriteByte(value)
				if err != nil {
					return nil, err
				}
				window[windowPos] = value
				windowPos = (windowPos + 1) & 0x0fff
			} else {
				var packedData uint16
				err = binary.Read(reader, binary.LittleEndian, &packedData)
				if err != nil {
					return nil, err
				}
				windowReadPos := packedData >> 4
				if windowReadPos == 0 {
					decompressed = writeBuf.Bytes()
					return decompressed, nil
				}
				count := (packedData & 0x0f) + 2
				for x := 0; x < int(count); x++ {
					err = writeBuf.WriteByte(window[windowReadPos])
					if err != nil {
						return nil, err
					}
					window[windowPos] = window[windowReadPos]
					windowReadPos = (windowReadPos + 1) & 0x0fff
					windowPos = (windowPos + 1) & 0x0fff
				}
			}
			tag = tag >> 1
		}
	}
	return
}

func decryptPacket(in []byte) (out []byte, err error) {
	out = make([]byte, len(in))
	for i := range in {
		out[i] = in[i]
	}
	if len(in) < 4 {
		out = append(out, '\x06')
		return
	}
	var (
		check   int
		checkAg uint16
	)
	for i := 3; i < len(in)-3; i++ {
		check = check + int(in[i])
		out[i] = in[i] ^ byte(i)
	}
	checkAg = binary.LittleEndian.Uint16(in[1:3])
	if check != int(checkAg) {
		return nil, errors.New("decrypt found error in checksum")
	}
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
func getGameOffset(rs io.ReadSeeker) int64 {
	n, err := rs.Seek(0, io.SeekCurrent)
	if err != nil {
		panic(err)
	}
	return n
}

func deserialize(move packetRec) (subs [][]byte, err error) {
	tmp := []byte{}
	if tmp[0] == 0x04 {
		tmp, err = decompressLZ77(move.Data, 1)
		if err != nil {
			return nil, errors.New("deserializer failed read")
		}
	}
	readPos := 0
	for {
		out := splitPacket(tmp[readPos:], false)
		if len(out) == 0 {
			break
		} else {
			readPos = len(out)
		}
		subs = append(subs, out)
	}
	return
}
func splitPacket(data []byte, smartpak bool) (out []byte) {
	if len(data) == 0 {
		out = []byte{}
		return
	}
	plGuide := map[byte]int{
		0x2: 13,
		0x6: 1,
		0x7: 1,
		0x20: 192,
		0x1a: 14,
		0x17: 2,
		0x18: 2,
		0x15: 1,
		0x8: 1,
		0x5: 65,
		'&': 41,
		'"': 6,
		'*': 2,
		0x1e: 2,
		',': int(data[1])+int(data[2])*256,
		0x09: 23,
		0x11: 4,
		0x10: 22,
		0x12: 5,
		0x0a: 7,
		0x28: 58,
		0x19: 3,
		0x0d: 36,
		0x0b: 9,
		0x0f: 6,
		0x0c: 11,
		0x1f: 5,
		0x23: 14,
		0x16: 17,
		0x1b: 6,
		0x29: 3,
		0x14: 24,
		0x21: 10,
		0x03: 7,
		0x0e: 14,
		0xff: 1,
		0xfe: 5,
		0xfd: (int(data[1])+int(data[2])*256)-4,
		0xf9: 73,
		0xfb: int(data[1])+3,
		0xfc: 5,
		0xfa: 1,
		0xf6: 1,
	}
	pl := plGuide[data[0]]
	if (data[0] == 0xff || data[0] == 0xfe || data[0] == 0xfd) && !smartpak {
		log.Warning("warning erroneous compression assumption")
	}
	if len(data) < pl {
		log.Warning("error subpacket longer than packet")
		pl = 0
	}
	if pl == 0 {
		out = []byte{}
		return
	} else {
		out = make([]byte, pl)
		dr := bytes.NewReader(data)
		bytesRead, err := dr.Read(out)
		if bytesRead != pl || err != nil {
			log.Fatalf("failed read for %02x packet", data[0])
		}
	}
	return
}
