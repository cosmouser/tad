package ted

import (
	"net"
	"os"
	"path"
	"strings"
	"testing"
)

var sample1 = path.Join("sample", "dckazikdidou.ted")

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
	clog, err := parseLobbyChat(tf)
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
	_, err = parseLobbyChat(tf)
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
	_, err = parseLobbyChat(tf)
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
	_, err = parseLobbyChat(tf)
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
