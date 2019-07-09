package tad

import (
	"bytes"
	"crypto/md5"
	"encoding/binary"
	"encoding/gob"
	"encoding/hex"
	"errors"
	log "github.com/sirupsen/logrus"
	"image"
	"io"
	"net"
	"os"
	"path"
	"strings"
	"testing"
)

var sample1 = path.Join("sample", "dckazikdidou.ted")
var sample2 = path.Join("sample", "dcfnhessano.ted")
var sample3 = path.Join("sample", "highground.ted")
var sample4 = path.Join("sample", "cheats.ted")
var sample5 = path.Join("sample", "dcfezkazik.ted")
var sample6 = path.Join("sample", "dcracefn0608.ted")
var sample7 = path.Join("sample", "dc3.ted")
var darkcometpng = path.Join("sample", "dc.png")
var testGif = path.Join("tmp", "test.gif")

const minuteInMilliseconds = 60000

// loadDemo is a function for conveniently opening up demo files and playing
// through their packets.
// It will need a reader to parse the file.
// It will need a function to use on the packets
// It needs a logging flag
func loadDemo(r io.ReadSeeker, testFunc func(packetRec, *game)) error {
	g := game{}
	sum, err := parseSummary(r)
	if err != nil {
		return err
	}
	g.MapName = string(bytes.Split(sum.MapName[:], []byte{0x0})[0])
	g.MaxUnits = int(sum.MaxUnits)
	g.Players = make([]DemoPlayer, int(sum.NumPlayers))
	eh, err := loadSection(r)
	if err != nil {
		return err
	}
	numSectors := int(eh[0])
	var playerAddrNum int
	for i := 0; i < numSectors; i++ {
		sec, err := loadSection(r)
		if err != nil {
			return err
		}
		extra, err := parseExtra(sec)
		if err != nil {
			return err
		}
		switch extra.sectorType {
		case commentsType:
			log.WithFields(log.Fields{
				"content": string(extra.data),
			}).Info("comment(s) detected")
		case lobbyChatType:
			lobbyChat, err := parseLobbyChat(extra)
			if err != nil {
				return err
			}
			g.LobbyChat = lobbyChat
		case versionNumberType:
			g.Version = string(extra.data)
		case dateStringType:
			g.RecDate = string(extra.data)
		case recFromType:
			g.RecFrom = string(extra.data)
		case playerAddrType:
			addr, err := parseAddressBlock(extra)
			if err != nil {
				return err
			}
			g.Players[playerAddrNum].IP = addr
			playerAddrNum++
		}
	}
	for i := 0; i < len(g.Players); i++ {
		player, err := parsePlayer(r)
		if err != nil {
			return err
		}
		g.Players[i].Color = player.Color
		g.Players[i].Side = player.Side
		g.Players[i].Number = player.Number
		g.Players[i].Name = string(bytes.TrimRight(player.Name[:], "\x00"))
	}
	for i := 0; i < len(g.Players); i++ {
		sm, err := parseStatMsg(r)
		if err != nil {
			return err
		}
		p, err := createPacket(sm.Data)
		if err != nil {
			return err
		}
		g.Players[i].Status = string(p)
		g.Players[i].Color = p[0x9e]
		if p[0xa2]&0x20 != 0 {
			g.Players[i].Cheats = true
		}
	}
	upd, err := parseUnitSyncData(r)
	if err != nil {
		return err
	}
	var updSum uint32
	for _, v := range upd {
		if v.InUse {
			updSum += v.ID + v.CRC
		}
	}
	var sumSlice bytes.Buffer
	if err := binary.Write(&sumSlice, binary.LittleEndian, updSum); err != nil {
		return err
	}
	sumArr := md5.Sum(sumSlice.Bytes())
	g.Unitsum = hex.EncodeToString(sumArr[:])
	gameOffset := getGameOffset(r)
	var loopCount int
	for err != io.EOF {
		pr := packetRec{}
		pr, err = loadMove(r)
		if pr.Sender > 10 || pr.Sender < 1 {
			if err != io.EOF {
				log.WithFields(log.Fields{
					"sender":    pr.Sender,
					"data":      pr.Data,
					"loopCount": loopCount,
				}).Warn("move from odd sender")
			}
		} else {
			g.TimeToDie[int(pr.Sender)-1] = loopCount
			loopCount++
		}
	}
	g.TotalMoves = loopCount
	nExpected, err := r.Seek(gameOffset, io.SeekStart)
	if err != nil || nExpected != gameOffset {
		return errors.New("seek to gameoffset failed")
	}
	var (
		lastDronePack   [10]uint32
		posSyncComplete [10]uint32
		recentPos       [10]bool
		lastSerial      [10]uint32
		masterHealth    saveHealth
	)
	masterHealth.MaxUnits = int32(g.MaxUnits)
	loopCount = 1
	for err != io.EOF && loopCount < g.TotalMoves {
		pr := packetRec{}
		pr, err = loadMove(r)
		if err != nil && err != io.EOF {
			return err
		}
		cpdb := make([]byte, len(pr.Data))
		for i := range pr.Data {
			cpdb[i] = pr.Data[i]
		}
		if recentPos[int(pr.Sender)-1] {
			recentPos[int(pr.Sender)-1] = false
			cpdb = unsmartpak(pr, &masterHealth, lastDronePack, false)
			posSyncComplete[int(pr.Sender)-1] = lastDronePack[int(pr.Sender)-1] + uint32(g.MaxUnits)
		}
		if lastDronePack[int(pr.Sender)-1] < posSyncComplete[int(pr.Sender)-1] {
			cpdb = unsmartpak(pr, &masterHealth, lastDronePack, false)
		} else {
			cpdb = unsmartpak(pr, &masterHealth, lastDronePack, true)
		}
		cpdb = append([]byte{cpdb[0], 'c', 'c', 0xff, 0xff, 0xff, 0xff}, cpdb[1:]...)
		if len(cpdb) > 7 {
			cpdb2 := append([]byte{}, cpdb[7:]...)
			for {
				tmp := splitPacket2(&cpdb2, false)
				// entry point for testFunc parameter
				msg := packetRec{
					Time:      pr.Time,
					Sender:    pr.Sender,
					IdemToken: pr.IdemToken,
					Data:      tmp,
				}
				testFunc(msg, &g)
				switch tmp[0] {
				case 0x2c:
					ip := binary.LittleEndian.Uint32(tmp[3:])
					lastSerial[int(pr.Sender)-1] = ip
				}
				if len(cpdb2) == 0 {
					break
				}

			}
		}
		loopCount++
	}
	return nil
}
func TestLoadDemo(t *testing.T) {
	tf, err := os.Open(sample1)
	if err != nil {
		t.Error(err)
	}
	counter := make(map[byte]int)
	err = loadDemo(tf, func(pr packetRec, g *game) {
		counter[pr.Data[0]]++
	})
	if err != nil {
		t.Error(err)
	}
	tf.Close()
}

// TestLoadSection opens a ted file and tests loading of multiple sections
func TestLoadSection(t *testing.T) {
	tf, err := os.Open(sample1)
	if err != nil {
		t.Error(err)
	}
	for i := 0; i < 10; i++ {
		data, err := loadSection(tf)
		if err != nil {
			t.Error(err)
		}
		t.Log(data)
	}
	tf.Close()
}

func TestParseSummary(t *testing.T) {
	tf, err := os.Open(sample1)
	if err != nil {
		t.Error(err)
	}
	s, err := parseSummary(tf)
	if err != nil {
		t.Error(err)
	}
	t.Logf("%+v", s)
	mapName := "[V] Dark Comet"
	if strings.Index(string(s.MapName[:]), mapName) != 0 {
		t.Errorf("wanted %v, got %v", mapName, string(s.MapName[:]))
	}
	tf.Close()
}

func TestParseLobbyChat(t *testing.T) {
	tf, err := os.Open(sample1)
	if err != nil {
		t.Error(err)
	}
	// skip the summary
	_, err = parseSummary(tf)
	if err != nil {
		t.Error(err)
	}
	// skip the extra header
	_, err = loadSection(tf)
	if err != nil {
		t.Error(err)
	}
	dat, err := loadSection(tf)
	if err != nil {
		t.Error(err)
	}
	ext1, err := parseExtra(dat)
	if err != nil {
		t.Error(err)
	}
	clog, err := parseLobbyChat(ext1)
	if err != nil {
		t.Error(err)
	}
	if len(clog) != 2 {
		t.Errorf("wanted 2 for length of Messages, got %d", len(clog))
	}
	if len(clog[0]) == 0 {
		t.Error("got 0 for length of first lobby message")
	}
	for i, v := range clog {
		t.Logf("message %d: %v", i, v)
	}
	tf.Close()
}
func TestPlaybackMessages(t *testing.T) {
	t.Skip()
	tf, err := os.Open(sample2)
	if err != nil {
		t.Error(err)
	}
	sum, err := parseSummary(tf)
	if err != nil {
		t.Error(err)
	}
	eh, err := loadSection(tf)
	numSectors := int(eh[0])
	for i := 0; i < numSectors; i++ {
		sec, err := loadSection(tf)
		if err != nil {
			t.Error(err)
		}
		_, err = parseExtra(sec)
		if err != nil {
			t.Error(err)
		}
	}
	// create players
	players := make([]DemoPlayer, int(sum.NumPlayers))
	for i := 0; i < int(sum.NumPlayers); i++ {
		player, err := parsePlayer(tf)
		if err != nil {
			t.Error(err)
		}
		players[i].Color = player.Color
		players[i].Side = player.Side
		players[i].Number = player.Number
		players[i].Name = string(bytes.TrimRight(player.Name[:], "\x00"))
	}
	for i := 0; i < int(sum.NumPlayers); i++ {
		sm, err := parseStatMsg(tf)
		if err != nil {
			t.Error(err)
		}
		players[i].Status = string(sm.Data)
		p, err := createPacket(sm.Data)
		if err != nil {
			t.Error(err)
		}
		idn, err := createIdent(p)
		t.Logf("%+v", idn)
		if err != nil {
			t.Error(err)
		}
		players[i].orgpid = idn.Player1
	}
	_, err = parseUnitSyncData(tf)
	if err != nil {
		t.Error(err)
	}
	gobf, err := os.Open("taesc900.gob")
	if err != nil {
		t.Error(err)
	}
	gd := gob.NewDecoder(gobf)
	unitmem := make(map[uint16]uint16)
	unitnames := make(map[uint16]string)
	err = gd.Decode(&unitnames)
	if err != nil {
		t.Error(err)
	}
	gobf.Close()
	playerMetadata := savePlayers{}
	var increment int
	for err != io.EOF {
		pr := packetRec{}
		pr, err = loadMove(tf)
		subpackets, err := deserialize(pr)
		if err != nil {
			t.Error(err)
		}
		for i := range subpackets {
			if os.Getenv("gamelogout") == "doit" {
				t.Log(playbackMsg(pr.Sender, subpackets[i], unitnames, unitmem))
			}
		}
		if pr.Sender > 10 || pr.Sender < 1 {
		} else {
			playerMetadata.TimeToDie[int(pr.Sender)-1] = increment
			increment++
		}
	}
	t.Logf("total moves: %d", increment)
	tf.Close()
}

func TestReadHeaders(t *testing.T) {
	tf, err := os.Open(sample1)
	if err != nil {
		t.Error(err)
	}
	sum, err := parseSummary(tf)
	if err != nil {
		t.Error(err)
	}
	want := "TA Demo\x00"
	if string(sum.Magic[:]) != want {
		t.Errorf("got %v or %v, wanted %v", string(sum.Magic[:]), sum.Magic[:], want)
	}
	version := int(sum.Version[0])
	if version != 5 {
		t.Error("got incompatible version number")
	}
	eh, err := loadSection(tf)
	numSectors := int(eh[0])
	for i := 0; i < numSectors; i++ {
		sec, err := loadSection(tf)
		if err != nil {
			t.Error(err)
		}
		_, err = parseExtra(sec)
		if err != nil {
			t.Error(err)
		}
	}
	// create players
	players := make([]DemoPlayer, int(sum.NumPlayers))
	for i := 0; i < int(sum.NumPlayers); i++ {
		player, err := parsePlayer(tf)
		if err != nil {
			t.Error(err)
		}
		if int(player.Number) != i+1 {
			t.Error("player out of order")
		}
		if i == 1 {
			playerName := "didou"
			if strings.Index(string(player.Name[:]), playerName) != 0 {
				t.Errorf("wanted %v, got %v", playerName, string(player.Name[:]))
			}
		}
		players[i].Color = player.Color
		players[i].Side = player.Side
		players[i].Number = player.Number
		players[i].Name = string(bytes.TrimRight(player.Name[:], "\x00"))
	}
	for i := 0; i < int(sum.NumPlayers); i++ {
		sm, err := parseStatMsg(tf)
		if err != nil {
			t.Error(err)
		}
		players[i].Status = string(sm.Data)
		p, err := createPacket(sm.Data)
		if err != nil {
			t.Error(err)
		}
		t.Logf("player %d has color %d", i, p[0x9e])
		idn, err := createIdent(p)
		if err != nil {
			t.Error(err)
		}
		t.Logf("%+v", idn)
		if i == 1 && (idn.Width != 2560 || idn.Height != 1440) {
			t.Error("failed to parseIdent properly")
		}
		players[i].orgpid = idn.Player1
	}
	upd, err := parseUnitSyncData(tf)
	if err != nil {
		t.Error(err)
	}
	if upd == nil {
		t.Error("got nil value for unit map")
	}
	t.Logf("len of upd: %v", len(upd))
	playerMetadata := savePlayers{}
	gameOffset := getGameOffset(tf)
	nExpected := 13841
	if int(gameOffset) != nExpected {
		t.Errorf("got %v for gameOffset, was expecting %v", gameOffset, nExpected)
	}
	var increment int
	for err != io.EOF {
		pr := packetRec{}
		pr, err = loadMove(tf)
		if pr.Sender > 10 || pr.Sender < 1 {
			t.Log("very odd")
		} else {
			playerMetadata.TimeToDie[int(pr.Sender)-1] = increment
			increment++
		}
	}
	totalMoves := increment
	t.Logf("total moves: %d", totalMoves)
	t.Logf("playerMetadata.TimeToDie: %v", playerMetadata.TimeToDie[:])
	nExpected2, err := tf.Seek(gameOffset, io.SeekStart)
	if err != nil || nExpected2 != gameOffset {
		t.Error("seek to gameOffset failed")
	}
	// playbackmsg section begin
	gobf, err := os.Open("taesc900.gob")
	if err != nil {
		t.Error(err)
	}
	gd := gob.NewDecoder(gobf)
	unitmem := make(map[uint16]uint16)
	unitnames := make(map[uint16]string)
	err = gd.Decode(&unitnames)
	if err != nil {
		t.Error(err)
	}
	gobf.Close()
	// playbackmsg section end
	pcps := make(map[byte]int)
	var maxunits uint32 = 1000
	var lastDronePack [10]uint32
	var posSyncComplete [10]uint32
	var recentPos [10]bool
	var lastSerial [10]uint32
	var masterHealth saveHealth
	masterHealth.MaxUnits = 1000
	increment = 1
	// make map of byte slices for 2c dump
	x2cSlices := make(map[byte][][]byte)
	for err != io.EOF && increment < totalMoves {
		pr := packetRec{}
		pr, err = loadMove(tf)
		if err != nil && err != io.EOF {
			t.Error(err)
		}
		// current packet data buffer
		cpdb := make([]byte, len(pr.Data))
		for i := range pr.Data {
			cpdb[i] = pr.Data[i]
		}
		// prevPack is a uint32 so lastDronePack ought to be [10]uint32
		// prevPack := lastDronePack[int(pr.Sender)-1]
		if recentPos[int(pr.Sender)-1] {
			recentPos[int(pr.Sender)-1] = false
			cpdb = unsmartpak(pr, &masterHealth, lastDronePack, false)
			posSyncComplete[int(pr.Sender)-1] = lastDronePack[int(pr.Sender)-1] + maxunits
		}
		if lastDronePack[int(pr.Sender)-1] < posSyncComplete[int(pr.Sender)-1] {
			cpdb = unsmartpak(pr, &masterHealth, lastDronePack, false)
		} else {
			cpdb = unsmartpak(pr, &masterHealth, lastDronePack, true)
		}
		cpdb = append([]byte{cpdb[0], 'c', 'c', 0xff, 0xff, 0xff, 0xff}, cpdb[1:]...)
		// fmMain.timemode.Checked section -- omitted
		// begin filtering information
		if len(cpdb) > 7 {
			cpdb2 := append([]byte{}, cpdb[7:]...)
			// cur only needed when re-packing and sending to server
			// cur := append([]byte{0x03, 0x00, 0x00}, cpdb[3:8]...)
			for {
				tmp := splitPacket2(&cpdb2, false)
				pcps[tmp[0]]++
				if tmp[0] != 0x2c || (tmp[0] == 0x2c && tmp[1] != 0x0b) {
					if os.Getenv("gamelogout") == "doit" {
						t.Log(playbackMsg(pr.Sender, tmp, unitnames, unitmem))
					}
				}
				switch tmp[0] {
				case 0x2c:
					ip := binary.LittleEndian.Uint32(tmp[3:])
					lastSerial[int(pr.Sender)-1] = ip

					// for map of byte slicesfor 2c dump
					if _, ok := x2cSlices[tmp[1]]; !ok {
						x2cSlices[tmp[1]] = [][]byte{}
					}
					x2cSlices[tmp[1]] = append(x2cSlices[tmp[1]], tmp)
				}
				// cur only needed when re-packing and sending to server
				// cur = append(cur, tmp...)
				if len(cpdb2) == 0 {
					break
				}

			}
		}
		increment++
	}
	for k, v := range pcps {
		t.Logf("%02x: %4d", k, v)
	}
	if pcps[0x09] <= 8 {
		t.Error("Expected more 0x09 packets")
	}
	if pcps[0x28] <= 59 {
		t.Error("Expected more 0x28 packets")
	}
	// // packet hunting section
	// // create packet dumps per type
	// for k := range x2cSlices {
	// 	fp, err := os.Create(path.Join("tmp", fmt.Sprintf("2c_%02x.hexdump", k)))
	// 	if err != nil {
	// 		t.Error(err)
	// 	}
	// 	for _, v := range x2cSlices[k] {
	// 		_, err := fp.Write(v)
	// 		if err != nil {
	// 			t.Error(err)
	// 		}
	// 	}
	// 	fp.Close()
	// }
	tf.Close()
}

func TestParseAddresses(t *testing.T) {
	tf, err := os.Open(sample1)
	if err != nil {
		t.Error(err)
	}
	// skip the summary
	sum, err := parseSummary(tf)
	if err != nil {
		t.Error(err)
	}
	// skip the extra header
	_, err = loadSection(tf)
	if err != nil {
		t.Error(err)
	}
	// skip lobbychat
	_, err = loadSection(tf)
	if err != nil {
		t.Error(err)
	}
	// skip version
	_, err = loadSection(tf)
	if err != nil {
		t.Error(err)
	}
	// skip date
	_, err = loadSection(tf)
	if err != nil {
		t.Error(err)
	}
	// skip startedfrom sector
	_, err = loadSection(tf)
	if err != nil {
		t.Error(err)
	}
	for i := 0; i < int(sum.NumPlayers); i++ {
		s, err := loadSection(tf)
		if err != nil {
			t.Error(err)
		}
		s2, err := parseExtra(s)
		if err != nil {
			t.Error(err)
		}
		addressBlock, err := parseAddressBlock(s2)
		if err != nil {
			t.Error(err)
		}
		if net.ParseIP(addressBlock) == nil {
			t.Error("unable to parse adddressBlock")
		}
	}
	tf.Close()
}

func TestParsePlayers(t *testing.T) {
	tf, err := os.Open(sample1)
	if err != nil {
		t.Error(err)
	}
	// skip the summary
	sum, err := parseSummary(tf)
	if err != nil {
		t.Error(err)
	}
	// skip the extra header
	_, err = loadSection(tf)
	if err != nil {
		t.Error(err)
	}
	// skip lobbychat
	_, err = loadSection(tf)
	if err != nil {
		t.Error(err)
	}
	// skip version
	_, err = loadSection(tf)
	if err != nil {
		t.Error(err)
	}
	// skip date
	_, err = loadSection(tf)
	if err != nil {
		t.Error(err)
	}
	// skip startedfrom sector
	_, err = loadSection(tf)
	if err != nil {
		t.Error(err)
	}
	for i := 0; i < int(sum.NumPlayers); i++ {
		_, err := loadSection(tf)
		if err != nil {
			t.Error(err)
		}
	}
	for i := 0; i < int(sum.NumPlayers); i++ {
		player, err := parsePlayer(tf)
		if err != nil {
			t.Error(err)
		}
		if int(player.Number) != i+1 {
			t.Error("player out of order")
		}
		if i == 1 {
			playerName := "didou"
			if strings.Index(string(player.Name[:]), playerName) != 0 {
				t.Errorf("wanted %v, got %v", playerName, string(player.Name[:]))
			}
		}

	}
	tf.Close()
}
func TestParseUnitSyncData(t *testing.T) {
	tf, err := os.Open(sample1)
	if err != nil {
		t.Error(err)
	}
	// skip the summary
	sum, err := parseSummary(tf)
	if err != nil {
		t.Error(err)
	}
	// skip the extra header
	_, err = loadSection(tf)
	if err != nil {
		t.Error(err)
	}
	// skip lobbychat
	_, err = loadSection(tf)
	if err != nil {
		t.Error(err)
	}
	// skip version
	_, err = loadSection(tf)
	if err != nil {
		t.Error(err)
	}
	// skip date
	_, err = loadSection(tf)
	if err != nil {
		t.Error(err)
	}
	// skip startedfrom sector
	_, err = loadSection(tf)
	if err != nil {
		t.Error(err)
	}
	for i := 0; i < int(sum.NumPlayers); i++ {
		_, err := loadSection(tf)
		if err != nil {
			t.Error(err)
		}
	}
	for i := 0; i < int(sum.NumPlayers); i++ {
		_, err := parsePlayer(tf)
		if err != nil {
			t.Error(err)
		}

	}
	for i := 0; i < int(sum.NumPlayers); i++ {
		_, err := loadSection(tf)
		if err != nil {
			t.Error(err)
		}

	}
	upd, err := parseUnitSyncData(tf)
	if err != nil {
		t.Error(err)
	}
	if upd == nil {
		t.Error("got nil value for upd map")
	}
	if v, ok := upd[0x5f958268]; !ok || v.InUse != false {
		t.Error("Helper unit marked as InUse or nil")
		t.Log(v)
	}
	if v, ok := upd[0x3ed65df7]; ok && v.InUse != true {
		t.Error("0x3ed65df7 unit marked as not InUse")
	}
	tf.Close()
}
func TestLoadDemoWithUnitmemAndNames(t *testing.T) {
	tf, err := os.Open(sample1)
	if err != nil {
		t.Error(err)
	}
	gobf, err := os.Open("taesc900.gob")
	if err != nil {
		t.Error(err)
	}
	gd := gob.NewDecoder(gobf)
	unitmem := make(map[uint16]uint16)
	unitnames := make(map[uint16]string)
	err = gd.Decode(&unitnames)
	if err != nil {
		t.Error(err)
	}
	gobf.Close()
	err = loadDemo(tf, func(pr packetRec, g *game) {
		if os.Getenv("playbackMsgs") == "enabled" {
			t.Log(playbackMsg(pr.Sender, pr.Data, unitnames, unitmem))
		}
	})
	if err != nil {
		t.Error(err)
	}
	tf.Close()
}
func TestDrawGif(t *testing.T) {
	tf, err := os.Open(sample7)
	if err != nil {
		t.Error(err)
	}
	if err != nil {
		t.Error(err)
	}
	// placeholder for map of unit positions
	frames := []playbackFrame{}
	unitmem := make(map[uint16]*taUnit)
	addFrame := func() {
		newFrame := playbackFrame{}
		newFrame.Number = len(frames)
		newFrame.Units = make(map[uint16]*taUnit)
		for k, v := range unitmem {
			newFrame.Units[k] = new(taUnit)
			newFrame.Units[k].Owner = v.Owner
			newFrame.Units[k].NetID = v.NetID
			newFrame.Units[k].Finished = v.Finished
			newFrame.Units[k].XPos = v.XPos
			newFrame.Units[k].YPos = v.YPos
			newFrame.Units[k].ZPos = v.ZPos
		}
		frames = append(frames, newFrame)
	}
	var clock, lastTime int
	var lastToken string
	var unitSpaces [10]uint16
	err = loadDemo(tf, func(pr packetRec, g *game) {
		if pr.IdemToken != lastToken {
			clock += int(pr.Time)
			lastToken = pr.IdemToken
		}
		if pr.Data[0] == 0x09 {
			tmp := &packet0x09{}
			if err := binary.Read(bytes.NewReader(pr.Data), binary.LittleEndian, tmp); err != nil {
				t.Error(err)
			}
			unitmem[tmp.UnitID] = &taUnit{
				Owner:    int(pr.Sender),
				NetID:    tmp.NetID,
				Finished: false,
				XPos:     int(tmp.XPos),
				YPos:     int(tmp.YPos),
				ZPos:     int(tmp.ZPos),
			}
			// check to see if its the first unit aka commander
			if int(tmp.UnitID)%g.MaxUnits == 1 {
				unitmem[tmp.UnitID].Finished = true
				unitSpaces[int(pr.Sender)-1] = tmp.UnitID
			}
		}
		if pr.Data[0] == 0x12 {
			tmp := &packet0x12{}
			if err := binary.Read(bytes.NewReader(pr.Data), binary.LittleEndian, tmp); err != nil {
				t.Error(err)
			}
			if tau, ok := unitmem[tmp.BuiltID]; ok && tau != nil {
				unitmem[tmp.BuiltID].Finished = true
			}
		}
		if pr.Data[0] == 0x0c {
			tmp := &packet0x0c{}
			if err := binary.Read(bytes.NewReader(pr.Data), binary.LittleEndian, tmp); err != nil {
				t.Error(err)
			}
			if tau, ok := unitmem[tmp.Destroyed]; ok || tau != nil {
				delete(unitmem, tmp.Destroyed)
			}
		}
		if pr.Data[0] == 0x0d {
			tmp := &packet0x0d{}
			if err := binary.Read(bytes.NewReader(pr.Data), binary.LittleEndian, tmp); err != nil {
				t.Error(err)
			}
			if tau, ok := unitmem[tmp.ShooterID]; ok && tau != nil {
				tau.XPos = int(tmp.OriginX)
				tau.YPos = int(tmp.OriginY)
				tau.ZPos = int(tmp.OriginZ)
			}
			if tau, ok := unitmem[tmp.ShotID]; ok && tau != nil {
				tau.XPos = int(tmp.DestX)
				tau.YPos = int(tmp.DestY)
				tau.ZPos = int(tmp.DestZ)
			}
		}
		if pr.Data[0] == 0x2c && len(pr.Data) >= 0x1a {
			// if 0x9: - 0xc00 isn't the unitid's netid, ignore
			x2cUnitID := binary.LittleEndian.Uint16(pr.Data[0x7:])
			x2cNetID := binary.LittleEndian.Uint16(pr.Data[0x9:])
			x2cXPos := binary.LittleEndian.Uint16(pr.Data[0xb:])
			x2cYPos := binary.LittleEndian.Uint16(pr.Data[0xd:])
			x2cUnitID += unitSpaces[int(pr.Sender)-1]
			if tau, ok := unitmem[x2cUnitID]; ok && tau != nil {
				if x2cNetID-0xc00 == tau.NetID {
					tau.XPos = int(x2cXPos) * 16
					tau.YPos = int(x2cYPos) * 16
				}
			}
		}
		if curTime := clock / 30000; curTime > lastTime {
			addFrame()
			lastTime = curTime
		}
	})
	if err != nil {
		t.Error(err)
	}
	out, err := os.Create(testGif)
	if err != nil {
		t.Error(err)
	}
	bgf, err := os.Open(darkcometpng)
	if err != nil {
		t.Error(err)
	}
	mapPic, picformat, err := image.Decode(bgf)
	if picformat != "png" || err != nil {
		if err != nil {
			t.Error(err)
		}
		t.Error("expected png format")
	}
	bgf.Close()
	// h:6144 w:7680
	mapRect := image.Rect(0, 0, 6144, 7680)
	err = drawGif(out, frames, mapPic, mapRect)
	if err != nil {
		t.Error(err)
	}
	out.Close()
	tf.Close()
}
