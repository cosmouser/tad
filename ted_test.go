package ted

import (
	"os"
	"path"
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
		t.Log(string(data))
	}
	tf.Close()
}

func TestParseSummary(t *testing.T) {
	tf, err := os.Open(sample1)
	if err != nil {
		t.Error(err)
	}
	data, err := loadSection(tf)
	if err != nil {
		t.Error(err)
	}
	s, err := parseSummary(data)
	if err != nil {
		t.Error(err)
	}
	mapName := "[V] Dark Comet"
	if string(s.MapName) != mapName {
		t.Errorf("wanted %v, got %v", mapName, string(s.MapName))
	}
	tf.Close()
}
