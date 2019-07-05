package tad

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"strings"
	"testing"
)

var sample1 = path.Join("sample", "dckazikdidou.ted")
var sample2 = path.Join("sample", "dcfnhessano.ted")

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
	if len(clog.Messages) != 2 {
		t.Errorf("wanted 2 for length of Messages, got %d", len(clog.Messages))
	}
	if len(clog.Messages[0]) == 0 {
		t.Error("got 0 for length of first lobby message")
	}
	for i, v := range clog.Messages {
		t.Logf("message %d: %v", i, v)
	}
	tf.Close()
}
func TestPlaybackMessages(t *testing.T) {
	// Skip playbacksection
	t.Skip()
	// --------------------

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
			t.Log(playbackMsg(pr.Sender, subpackets[i], unitnames, unitmem))
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
		idn, err := createIdent(p)
		if err != nil {
			t.Error(err)
		}
		// t.Logf("%+v", idn)
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
	pcps := make(map[byte]int)
	var maxunits uint32 = 1000
	var lastDronePack [10]uint32
	var posSyncComplete [10]uint32
	var recentPos [10]bool
	var lastSerial [10]uint32
	var masterHealth saveHealth
	masterHealth.MaxUnits = 1000
	increment = 1
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
				switch tmp[0] {
				case 0x2c:
					ip := binary.LittleEndian.Uint32(tmp[3:])
					lastSerial[int(pr.Sender)-1] = ip
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
	if os.Getenv("packet_hunting") == "hexdumps" {
		// packet hunting section
		// create packet dumps per type
		fds := make(map[byte]*os.File)
		for k, v := range pcps {
			fp, err := os.Create(path.Join("tmp", fmt.Sprintf("%02x_%d.hexdump", k, v)))
			if err != nil {
				t.Error(err)
			}
			fds[k] = fp
		}
		nExpected2, err = tf.Seek(gameOffset, io.SeekStart)
		if err != nil || nExpected2 != gameOffset {
			t.Error("seek to gameOffset failed")
		}
		for err != io.EOF {
			pr := packetRec{}
			pr, err = loadMove(tf)
			if len(pr.Data) > 0 {
				subpackets, err := deserialize(pr)
				if err != nil {
					t.Error(err)
				}
				for i := range subpackets {
					_, err := fds[subpackets[i][0]].Write(subpackets[i])
					if err != nil {
						t.Error(err)
					}
				}
			}
		}
		for _, v := range fds {
			v.Close()
		}
	}
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
		addressBlock, err := parseAddressBlock(tf)
		if err != nil {
			t.Error(err)
		}
		if net.ParseIP(addressBlock.IP) == nil {
			t.Error("unable to parse adddressBlock.IP")
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
		_, err := parseAddressBlock(tf)
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
		_, err := parseAddressBlock(tf)
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
