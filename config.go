package sardine

import (
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
				c.CheckPlugins[id] = &CheckPlugin{Command: pc.Command}
			}
		default:
			return nil, fmt.Errorf("Unknown config section [plugin.%s]", key)
		}
	}
	c.Plugin = nil
	return &c, nil
}
