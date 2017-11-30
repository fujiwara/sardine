package sardine

import (
	"fmt"
	"time"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Region        string
	Plugin        map[string]map[string]*PluginConfig
	MetricPlugins map[string]*MetricPlugin
	CheckPlugins  map[string]*CheckPlugin
}

type PluginConfig struct {
	Command string
	Timeout time.Duration
}

func (pc *PluginConfig) NewMetricPlugin(id string) *MetricPlugin {
	mp := &MetricPlugin{
		ID:      fmt.Sprintf("plugin.metrics.%s", id),
		Command: pc.Command,
		Timeout: pc.Timeout,
	}
	if mp.Timeout == 0 {
		mp.Timeout = defaultCommandTimeout
	}
	return mp
}

var defaultCommandTimeout = 60 * time.Second

func LoadConfig(path string) (*Config, error) {
	c := Config{
		MetricPlugins: make(map[string]*MetricPlugin),
		CheckPlugins:  make(map[string]*CheckPlugin),
	}
	_, err := toml.DecodeFile(path, &c)
	if err != nil {
		return nil, err
	}
	for key, value := range c.Plugin {
		switch key {
		case "metrics":
			for id, pc := range value {
				c.MetricPlugins[id] = pc.NewMetricPlugin(id)
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
