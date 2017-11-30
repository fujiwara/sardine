package sardine_test

import (
	"testing"

	"github.com/fujiwara/sardine"
)

func TestLoadConfig(t *testing.T) {
	c, err := sardine.LoadConfig("test/config.toml")
	if err != nil {
		t.Error(err)
	}
	t.Logf("%#v", c)
}
