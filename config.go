package sardine

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	config "github.com/kayac/go-config"
)

type Config struct {
	Plugin        map[string]map[string]*PluginConfig
	MetricPlugins map[string]*MetricPlugin
	CheckPlugins  map[string]*CheckPlugin
}

type PluginConfig struct {
	Namespace  string
	Command    string
	Timeout    time.Duration
	Dimensions []*Dimension
}

type Dimension string

func (d *Dimension) CloudWatchDimentions() ([]*cloudwatch.Dimension, error) {
	var ds []*cloudwatch.Dimension
	for _, df := range strings.Split(string(*d), ",") {
		cols := strings.SplitN(df, "=", 2)
		if len(cols) != 2 {
			return nil, fmt.Errorf("invalid dimention: %s", df)
		}
		ds = append(ds,
			&cloudwatch.Dimension{
				Name:  aws.String(cols[0]),
				Value: aws.String(cols[1]),
			},
		)
	}
	return ds, nil
}

func (pc *PluginConfig) NewMetricPlugin(id string) (*MetricPlugin, error) {
	if pc.Command == "" {
		return nil, errors.New("command required")
	}
	mp := &MetricPlugin{
		ID:      fmt.Sprintf("plugin.metrics.%s", id),
		Command: pc.Command,
		Timeout: pc.Timeout,
	}
	for _, d := range pc.Dimensions {
		if ds, err := d.CloudWatchDimentions(); err != nil {
			return nil, err
		} else {
			mp.Dimensions = append(mp.Dimensions, ds)
		}
	}
	if mp.Timeout == 0 {
		mp.Timeout = defaultCommandTimeout
	}
	return mp, nil
}

func (pc *PluginConfig) NewCheckPlugin(id string) (*CheckPlugin, error) {
	if pc.Namespace == "" {
		return nil, errors.New("namespace required")
	}
	if pc.Command == "" {
		return nil, errors.New("command required")
	}

	cp := &CheckPlugin{
		ID:        fmt.Sprintf("plugin.check.%s", id),
		Namespace: pc.Namespace,
		Command:   pc.Command,
		Timeout:   pc.Timeout,
	}
	for _, d := range pc.Dimensions {
		if ds, err := d.CloudWatchDimentions(); err != nil {
			return nil, err
		} else {
			cp.Dimensions = append(cp.Dimensions, ds)
		}
	}
	if cp.Timeout == 0 {
		cp.Timeout = defaultCommandTimeout
	}
	return cp, nil
}

var defaultCommandTimeout = 60 * time.Second

func LoadConfig(path string) (*Config, error) {
	c := Config{
		MetricPlugins: make(map[string]*MetricPlugin),
		CheckPlugins:  make(map[string]*CheckPlugin),
	}
	if err := config.LoadWithEnvTOML(&c, path); err != nil {
		return nil, err
	}
	for key, value := range c.Plugin {
		switch key {
		case "metrics":
			for id, pc := range value {
				mp, err := pc.NewMetricPlugin(id)
				if err != nil {
					return nil, err
				}
				c.MetricPlugins[id] = mp
			}
		case "check":
			for id, pc := range value {
				cp, err := pc.NewCheckPlugin(id)
				if err != nil {
					return nil, err
				}
				c.CheckPlugins[id] = cp
			}
		default:
			return nil, fmt.Errorf("unknown config section [plugin.%s]", key)
		}
	}
	c.Plugin = nil
	return &c, nil
}
