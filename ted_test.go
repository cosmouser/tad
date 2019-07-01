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
