package tad

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	lcs "github.com/yudai/golcs"
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

func parseLobbyChat(extra extraSector) (messages []string, err error) {
	raw := bytes.Split(extra.data, []byte{0x0d})
	messages = make([]string, len(raw)-1)
	for i := range messages {
		messages[i] = string(raw[i])
	}
	return
}

func parseAddressBlock(extra extraSector) (ab string, err error) {
	addressData := simpleCrypt(extra.data)
	ip := bytes.Split(addressData[0x50:], []byte{0x0})
	ab = string(ip[0])
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

func loadMove(r io.Reader) (pr PacketRec, err error) {
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
	pr.IdemToken = uuid.New().String()
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

func deserialize(move PacketRec) (subs [][]byte, err error) {
	if len(move.Data) < 1 {
		return
	}
	tmp := make([]byte, len(move.Data))
	for i := range move.Data {
		tmp[i] = move.Data[i]
	}
	if tmp[0] == 0x04 {
		tmp, err = decompressLZ77(move.Data, 1)
		if err != nil {
			return nil, errors.New("deserializer failed read")
		}
	}
	readPos := 1
	for {
		out := splitPacket(tmp[readPos:])
		if len(out) == 0 {
			break
		} else {
			readPos += len(out)
		}
		subs = append(subs, out)
	}
	return
}
func playbackMsg(sender byte, data []byte, names map[uint16]string, unitmem map[uint16]uint16) string {
	tap, err := loadTAPacket(data)
	if err != nil && err != io.EOF {
		log.Fatal(err)
	}
	if err != nil {
		return ""
	}
	msg := fmt.Sprintf("player %d sent %v", sender, tap.printMessage(names, unitmem))
	switch tap.GetMarker() {
	case 0x09:
		unitID := tap.(*packet0x09).UnitID
		netID := tap.(*packet0x09).NetID
		unitmem[unitID] = netID
	}
	return msg
}

func loadTAPacket(pdata []byte) (taPacket, error) {
	pr := bytes.NewReader(pdata)
	switch pdata[0] {
	case 0x28:
		tmp := &packet0x28{}
		err := binary.Read(pr, binary.LittleEndian, tmp)
		if err != nil {
			return tmp, err
		}
		return tmp, nil
	case 0x09:
		tmp := &packet0x09{}
		err := binary.Read(pr, binary.LittleEndian, tmp)
		if err != nil {
			return tmp, err
		}
		return tmp, nil
	case 0x0c:
		tmp := &packet0x0c{}
		err := binary.Read(pr, binary.LittleEndian, tmp)
		if err != nil {
			return tmp, err
		}
		return tmp, nil
	case 0xfc:
		tmp := &packet0xfc{}
		err := binary.Read(pr, binary.LittleEndian, tmp)
		if err != nil {
			return tmp, err
		}
		return tmp, nil
	case 0x12:
		tmp := &packet0x12{}
		err := binary.Read(pr, binary.LittleEndian, tmp)
		if err != nil {
			return tmp, err
		}
		return tmp, nil
	case 0x0b:
		tmp := &packet0x0b{}
		err := binary.Read(pr, binary.LittleEndian, tmp)
		if err != nil {
			return tmp, err
		}
		return tmp, nil
	case 0x05:
		tmp := &packet0x05{}
		err := binary.Read(pr, binary.LittleEndian, tmp)
		if err != nil {
			return tmp, err
		}
		return tmp, nil
	case 0x11:
		tmp := &packet0x11{}
		err := binary.Read(pr, binary.LittleEndian, tmp)
		if err != nil {
			return tmp, err
		}
		return tmp, nil
	}
	tmp := &packetDefault{}
	b, err := pr.ReadByte()
	if err != nil {
		return tmp, err
	}
	tmp.Marker = b
	tmp.Data = make([]byte, len(pdata)-1)
	_, err = pr.Read(tmp.Data)
	if err != nil {
		return tmp, err
	}
	return tmp, nil
}
func unsmartpak(pr PacketRec, save *saveHealth, last2cs [10]uint32, incnon2c bool) []byte {
	var packnum uint32
	var ut []byte
	var packout bytes.Buffer
	c := []byte(string(pr.Data[0]) + "xx" + string(pr.Data[1:]))
	if c[0] == 0x04 {
		ctmp, err := decompressLZ77([]byte(c), 3)
		if err != nil {
			log.Fatal(err)
		}
		c = ctmp
	}
	c = c[3:]
	for {
		s := splitPacket2(&c, true)
		switch s[0] {
		case 0xfe:
			packnum = binary.LittleEndian.Uint32(s[1:])
			last2cs[int(pr.Sender)-1] = packnum
		case 0xff:
			err := binary.Write(&packout, binary.LittleEndian, packnum)
			if err != nil {
				log.Fatal(err)
			}
			packoutData := packout.Bytes()
			packout.Reset()
			tmp := append([]byte{0x2c, 0x0b, 0x00}, append(packoutData, 0xff, 0xff, 0x01, 0x00)...)
			packnum++
			last2cs[int(pr.Sender)-1] = packnum
			ut = append(ut, tmp...)
		case 0xfd:
			err := binary.Write(&packout, binary.LittleEndian, packnum)
			if err != nil {
				log.Fatal(err)
			}
			packoutData := packout.Bytes()
			packout.Reset()
			tmp := append(s[:3], append(packoutData, s[3:]...)...)
			rw := binary.LittleEndian.Uint16(tmp[7:])
			if rw == 0xffff {
				nh := binary.LittleEndian.Uint32(tmp[10:])
				save.Health[int(packnum%uint32(save.MaxUnits))] = int32(nh)
			}
			packnum++
			last2cs[int(pr.Sender)-1] = packnum
			tmp[0] = 0x2c
			ut = append(ut, tmp...)
		case 0x2c:
			last2cs[int(pr.Sender)-1] = binary.LittleEndian.Uint32(s[3:])
		default:
			if incnon2c {
				ut = append(ut, s...)
			}
		}
		if len(c) == 0 {
			return append([]byte{0x3}, ut...)
		}
	}
}

func splitPacket2(data *[]byte, smartpak bool) (out []byte) {
	var (
		length int
		tmp    []byte
	)
	if len(*data) == 0 {
		out = []byte{}
		return
	}
	tmp = append([]byte{}, *data...)

	plGuide := map[byte]int{
		0x2:  13,
		0x6:  1,
		0x7:  1,
		0x20: 192,
		0x1a: 14,
		0x17: 2,
		0x18: 2,
		0x15: 1,
		0x8:  1,
		0x5:  65,
		'&':  41,
		'"':  6,
		0x2a: 2,
		0x1e: 2,
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
		0xf9: 73,
		0xfc: 5,
		0xfa: 1,
		0xf6: 1,
	}
	if len(*data) > 2 {
		plGuide[','] = int((*data)[1]) + int((*data)[2])*256
		plGuide[0xfd] = (int((*data)[1]) + int((*data)[2])*256) - 4
		plGuide[0xfb] = int((*data)[1]) + 3
	}
	length = plGuide[(*data)[0]]
	if ((*data)[0] == 0xff || tmp[0] == 0xfe || tmp[0] == 0xfd) && !smartpak {
		log.Warning("erroneous compression assumption")
	}
	if len(tmp) < length {
		log.Error("subpacket longer than packet")
		length = 0
	}
	if length == 0 {
		log.Info("empty packet")
		*data = []byte{}
		out = tmp
	} else {
		*data = append([]byte{}, tmp[length:]...)
		out = append([]byte{}, tmp[:length]...)
	}
	return
}
func splitPacket(data []byte) (out []byte) {
	if len(data) == 0 {
		out = []byte{}
		return
	}
	plGuide := map[byte]int{
		0x2:  13,
		0x6:  1,
		0x7:  1,
		0x20: 192,
		0x1a: 14,
		0x17: 2,
		0x18: 2,
		0x15: 1,
		0x8:  1,
		0x5:  65,
		'&':  41,
		'"':  6,
		'*':  2,
		0x1e: 2,
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
		0xf9: 73,
		0xfc: 5,
		0xfa: 1,
		0xf6: 1,
	}
	if len(data) > 2 {
		plGuide[','] = int(data[1]) + int(data[2])*256
		plGuide[0xfd] = (int(data[1]) + int(data[2])*256) - 4
		plGuide[0xfb] = int(data[1]) + 3
	}
	pl := plGuide[data[0]]
	if pl == 0 {
		out = []byte{}
		return
	}
	out = make([]byte, pl)
	dr := bytes.NewReader(data)
	bytesRead, err := dr.Read(out)
	if bytesRead != pl || err != nil {
		log.Fatalf("failed read for %02x packet", data[0])
	}
	return
}
func appendDiffData(ds *[]interface{}, pr PacketRec) error {
	switch pr.Data[0] {
	case 0xd:
		tmp := &packet0x0d{}
		if err := binary.Read(bytes.NewReader(pr.Data), binary.LittleEndian, tmp); err != nil {
			return err
		}
		*ds = append(*ds, tmp.OriginX, tmp.OriginZ, tmp.OriginY, tmp.DestX, tmp.DestZ, tmp.DestY)
	case 0x11:
		tmp := &packet0x11{}
		if err := binary.Read(bytes.NewReader(pr.Data), binary.LittleEndian, tmp); err != nil {
			return err
		}
		*ds = append(*ds, tmp.State)
	case 0xb:
		tmp := &packet0x0b{}
		if err := binary.Read(bytes.NewReader(pr.Data), binary.LittleEndian, tmp); err != nil {
			return err
		}
		*ds = append(*ds, tmp.Damage)
	case 0x5:
		tmp := &packet0x05{}
		if err := binary.Read(bytes.NewReader(pr.Data), binary.LittleEndian, tmp); err != nil {
			return err
		}
		*ds = append(*ds, string(tmp.Message[:]))
	case 0x9:
		tmp := &packet0x09{}
		if err := binary.Read(bytes.NewReader(pr.Data), binary.LittleEndian, tmp); err != nil {
			return err
		}
		*ds = append(*ds, tmp.NetID, tmp.XPos, tmp.ZPos, tmp.YPos)
	case 0xfc:
		tmp := &packet0xfc{}
		if err := binary.Read(bytes.NewReader(pr.Data), binary.LittleEndian, tmp); err != nil {
			return err
		}
		*ds = append(*ds, tmp.XPos, tmp.YPos)
	}
	return nil
}
func diffDataSeries(s1 []interface{}, s2 []interface{}) float64 {
	shared := lcs.New(s1, s2)
	return float64(shared.Length()) / float64(len(s1))
}
func (gp *game) getParty() string {
	nameToNumber := make(map[string]int)
	names := make([]string, len(gp.Players))
	for i := range gp.Players {
		names[i] = gp.Players[i].Name
		nameToNumber[names[i]] = i
	}
	sort.Strings(names)
	party := gp.MapName
	for _, n := range names {
		tmp := fmt.Sprintf("%v%v%v%v%v",
			gp.Players[nameToNumber[n]].Name,
			gp.Players[nameToNumber[n]].Side,
			gp.Players[nameToNumber[n]].Color,
			gp.Players[nameToNumber[n]].IP,
			gp.Players[nameToNumber[n]].TDPID,
		)
		party += tmp
	}
	return fmt.Sprintf("%x", sha1.Sum([]byte(party)))
}

// GenPnames creates a non-alphabetical map of packet from to player name
func GenPnames(players []DemoPlayer) map[byte]string {
	pnames := make(map[byte]string)
	for i := range players {
		pnames[byte(players[i].Number)] = players[i].Name
	}
	return pnames
}
func getFinalScores(list []PacketRec, pnameMap map[byte]string) (finalScores []FinalScore, err error) {
	var sp packet0x28
	var c int
	smap := make(map[byte]int)
	for k := range pnameMap {
		smap[k] = c
		c++
	}
	finalScores = make([]FinalScore, c)
	for k := range pnameMap {
		finalScores[smap[k]].Player = pnameMap[k]
	}
	foulPlay := &scoreError{}
	for _, pr := range list {
		if _, ok := pnameMap[pr.Sender]; ok && pr.Data[0] == 0x28 {
			err = binary.Read(bytes.NewReader(pr.Data), binary.LittleEndian, &sp)
			if err != nil {
				return nil, err
			}
			if int(sp.Losses) < finalScores[smap[pr.Sender]].Losses {
				foulPlay = &scoreError{player: pnameMap[pr.Sender], playerNumber: int(pr.Sender)}
				log.Printf("foul play found from %v", pnameMap[pr.Sender])
			}
			if int(sp.Kills) < finalScores[smap[pr.Sender]].Kills {
				foulPlay = &scoreError{player: pnameMap[pr.Sender], playerNumber: int(pr.Sender)}
				log.Printf("foul play found from %v", pnameMap[pr.Sender])
			}
			finalScores[smap[pr.Sender]].Status = int(sp.Status)
			finalScores[smap[pr.Sender]].Won = int(sp.ComKills)
			finalScores[smap[pr.Sender]].Lost = int(sp.ComLosses)
			finalScores[smap[pr.Sender]].Kills = int(sp.Kills)
			finalScores[smap[pr.Sender]].Losses = int(sp.Losses)
			finalScores[smap[pr.Sender]].TotalE = float64(sp.TotalE)
			finalScores[smap[pr.Sender]].ExcessE = float64(sp.ExcessE)
			finalScores[smap[pr.Sender]].TotalM = float64(sp.TotalM)
			finalScores[smap[pr.Sender]].ExcessM = float64(sp.ExcessM)
		}
	}
	if foulPlay.playerNumber != 0 {
		err = foulPlay
	}
	return
}

// GenScoreSeries extracts the series of 0x28 packets from the game
func GenScoreSeries(list []PacketRec, pnameMap map[byte]string) (series map[string][]SPLite, err error) {
	series = make(map[string][]SPLite)
	seriesFull := make(map[string][]packet0x28)
	var (
		scorePacket packet0x28
		litePacket  SPLite
		ediff       float64
		mdiff       float64
		tdiff       float64
	)
	var clock int
	var lastToken string
	for _, pr := range list {
		if pr.IdemToken != lastToken {
			clock += int(pr.Time)
			lastToken = pr.IdemToken
		}
		if pr.Data[0] == 0x28 {
			err = binary.Read(bytes.NewReader(pr.Data), binary.LittleEndian, &scorePacket)
			if err != nil {
				return nil, err
			}
			if len(series[pnameMap[pr.Sender]]) == 0 {
				series[pnameMap[pr.Sender]] = append(series[pnameMap[pr.Sender]], SPLite{Milliseconds: clock})
			}
			if len(seriesFull[pnameMap[pr.Sender]]) == 0 {
				seriesFull[pnameMap[pr.Sender]] = append(seriesFull[pnameMap[pr.Sender]], packet0x28{})
			}
			ediff = float64(scorePacket.TotalE - seriesFull[pnameMap[pr.Sender]][len(seriesFull[pnameMap[pr.Sender]])-1].TotalE)
			mdiff = float64(scorePacket.TotalM - seriesFull[pnameMap[pr.Sender]][len(seriesFull[pnameMap[pr.Sender]])-1].TotalM)
			tdiff = float64(clock - series[pnameMap[pr.Sender]][len(series[pnameMap[pr.Sender]])-1].Milliseconds)
			litePacket.Energy = (ediff / tdiff) * 1000
			litePacket.Metal = (mdiff / tdiff) * 1000
			litePacket.Kills = int(scorePacket.Kills)
			litePacket.Losses = int(scorePacket.Losses)
			litePacket.TotalE = float64(scorePacket.TotalE)
			litePacket.TotalM = float64(scorePacket.TotalM)
			litePacket.ExcessE = float64(scorePacket.ExcessE)
			litePacket.ExcessM = float64(scorePacket.ExcessM)
			if math.IsNaN(litePacket.Energy) || math.IsInf(litePacket.Energy, 1) {
				litePacket.Energy = 1
			}
			if math.IsNaN(litePacket.Metal) || math.IsInf(litePacket.Metal, 1) {
				litePacket.Metal = 1
			}
			litePacket.Milliseconds = clock
			if litePacket.Metal > 1.0 || litePacket.Energy > 1.0 {
				series[pnameMap[pr.Sender]] = append(series[pnameMap[pr.Sender]], litePacket)
			}
			seriesFull[pnameMap[pr.Sender]] = append(seriesFull[pnameMap[pr.Sender]], scorePacket)
		}
	}
	return
}

func getTeams(list []PacketRec, gp *game) (allies []int, err error) {
	// If a player allies another player and that player allies them back
	// they are allies. If a player unallies a player they are no longer allies.
	alliedTimer := make([]int, 10)
	alliedTo := make([]bool, 10)
	alliedBy := make([]bool, 10)
	tdpidMap := make(map[int32]byte)
	for _, p := range gp.Players {
		tdpidMap[p.TDPID] = p.Number - 1
	}
	var moveCounter int
	var lastToken string
	for _, pr := range list {
		if pr.IdemToken != lastToken {
			moveCounter++
			lastToken = pr.IdemToken
		}
		if pr.Data[0] == 0x23 {
			tmp := packet0x23{}
			err = binary.Read(bytes.NewReader(pr.Data), binary.LittleEndian, &tmp)
			if err != nil {
				return
			}
			if tmp.Status == 1 {
				if tdpidMap[tmp.Player] == 0 {
					alliedTo[tdpidMap[tmp.Allied]] = true
				} else {
					alliedBy[tdpidMap[tmp.Player]] = true
				}
			} else {
				if tdpidMap[tmp.Player] == 0 {
					alliedTo[tdpidMap[tmp.Allied]] = false
				} else {
					alliedBy[tdpidMap[tmp.Player]] = false
				}
			}
		}
		for i := range alliedTo {
			if alliedTo[i] && alliedBy[i] {
				alliedTimer[i]++
			}
		}
	}
	for i := range alliedTimer {
		if float64(alliedTimer[i])/float64(gp.TotalMoves) > 0.80 {
			allies = append(allies, i)
		}
	}
	return
}
