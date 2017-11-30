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
	mp := c.MetricPlugins["memcached"]
	if mp.Command != "mackerel-plugin-memcached --host 127.0.0.1 --port 11211" {
		t.Error("unexpected command", mp.Command)
	}
	if len(mp.Dimensions) != 2 {
		t.Errorf("unexpected dimensions len expected:2 got:%d", len(mp.Dimensions))
	}
}

func TestDimention(t *testing.T) {
	d := sardine.Dimension("Foo=bar,Bar=baz")
	ds, err := d.CloudWatchDimentions()
	if err != nil {
		t.Error(err)
	}
	if len(ds) != 2 {
		t.Errorf("unexpected dimensions len expected:2 got:%d", len(ds))
	}
	n1, v1 := ds[0].Name, ds[0].Value
	if *n1 != "Foo" || *v1 != "bar" {
		t.Errorf("unexpected dimention[0] expected:Foo=bar got:%s=%s", *n1, *v1)
	}

	n2, v2 := ds[1].Name, ds[1].Value
	if *n2 != "Bar" || *v2 != "baz" {
		t.Errorf("unexpected dimention[1] expected:Bar=baz got:%s=%s", *n2, *v2)
	}
}
