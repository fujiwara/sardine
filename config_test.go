package sardine_test

import (
	"testing"
	"time"

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
	if mp.Interval != 10*time.Second {
		t.Errorf("unexpected interval expected:10s got:%s", mp.Interval)
	}
	if mp.Timeout != 15*time.Second {
		t.Errorf("unexpected timeout expected:15s got:%s", mp.Timeout)
	}

	cp := c.CheckPlugins["memcached"]
	if cp.Command != "sh -c 'echo version | nc 127.0.0.1 11211'" {
		t.Error("unexpected command", mp.Command)
	}
	if cp.Namespace != "memcached/check" {
		t.Error("unexpected namespace", cp.Namespace)
	}
	if len(cp.Dimensions) != 0 {
		t.Errorf("unexpected dimensions len expected:0 got:%d", len(cp.Dimensions))
	}
	if cp.Interval != time.Minute {
		t.Errorf("unexpected interval expected:1m got:%s", cp.Interval)
	}
	if cp.Timeout != time.Minute {
		t.Errorf("unexpected timeout expected:1m got:%s", cp.Timeout)
	}
}

func TestDimension(t *testing.T) {
	d := sardine.Dimension("Foo=bar,Bar=baz")
	ds, err := d.CloudWatchDimensions()
	if err != nil {
		t.Error(err)
	}
	if len(ds) != 2 {
		t.Errorf("unexpected dimensions len expected:2 got:%d", len(ds))
	}
	n1, v1 := ds[0].Name, ds[0].Value
	if *n1 != "Foo" || *v1 != "bar" {
		t.Errorf("unexpected dimension[0] expected:Foo=bar got:%s=%s", *n1, *v1)
	}

	n2, v2 := ds[1].Name, ds[1].Value
	if *n2 != "Bar" || *v2 != "baz" {
		t.Errorf("unexpected dimension[1] expected:Bar=baz got:%s=%s", *n2, *v2)
	}
}
