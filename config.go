package sardine

import (
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	config "github.com/kayac/go-config"
	"github.com/pkg/errors"
)

type Config struct {
	APIKey                  string
	Plugin                  map[string]map[string]*PluginConfig
	CheckPlugins            map[string]*CheckPlugin
	CloudWatchMetricPlugins map[string]*CloudWatchMetricPlugin
	MackerelMetricPlugins   map[string]*MackerelMetricPlugin
}

type duration struct {
	time.Duration
}

func (d *duration) UnmarshalText(text []byte) error {
	var err error
	d.Duration, err = time.ParseDuration(string(text))
	return err
}

type PluginConfig struct {
	Namespace   string
	Command     string
	Timeout     duration
	Interval    duration
	Dimensions  []*Dimension
	Destination string
	Service     string
}

type Dimension string

func (d *Dimension) CloudWatchDimensions() ([]*cloudwatch.Dimension, error) {
	var ds []*cloudwatch.Dimension
	for _, df := range strings.Split(string(*d), ",") {
		cols := strings.SplitN(df, "=", 2)
		if len(cols) != 2 {
			return nil, fmt.Errorf("invalid dimension: %s", df)
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

func (pc *PluginConfig) NewCloudWatchDriver(id string) (*CloudWatchMetricPlugin, error) {
	if pc.Command == "" {
		return nil, errors.New("command required")
	}
	dimensions := [][]*cloudwatch.Dimension{}
	for _, d := range pc.Dimensions {
		if ds, err := d.CloudWatchDimensions(); err != nil {
			return nil, err
		} else {
			dimensions = append(dimensions, ds)
		}
	}
	mp := MetricPlugin{
		ID:       fmt.Sprintf("plugin.metrics.%s", id),
		Command:  pc.Command,
		Timeout:  pc.Timeout.Duration,
		Interval: pc.Interval.Duration,
	}
	pd := &CloudWatchMetricPlugin{Dimensions: dimensions, MetricPlugin: mp}

	mp.PluginDriver = pd
	if mp.Timeout == 0 {
		mp.Timeout = DefaultCommandTimeout
	}
	if mp.Interval == 0 {
		mp.Interval = DefaultInterval
	}
	return pd, nil
}

func (pc *PluginConfig) NewMackerelDriver(id string) (*MackerelMetricPlugin, error) {
	if pc.Command == "" {
		return nil, errors.New("command required")
	}
	if pc.Service == "" {
		return nil, errors.New("service required")
	}
	mp := MetricPlugin{
		ID:       fmt.Sprintf("plugin.servicemetrics.%s", id),
		Command:  pc.Command,
		Timeout:  pc.Timeout.Duration,
		Interval: pc.Interval.Duration,
	}

	pd := &MackerelMetricPlugin{Service: pc.Service, MetricPlugin: mp}

	mp.PluginDriver = pd

	if mp.Timeout == 0 {
		mp.Timeout = DefaultCommandTimeout
	}
	if mp.Interval == 0 {
		mp.Interval = DefaultInterval
	}
	return pd, nil
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
		Timeout:   pc.Timeout.Duration,
		Interval:  pc.Interval.Duration,
	}
	for _, d := range pc.Dimensions {
		if ds, err := d.CloudWatchDimensions(); err != nil {
			return nil, err
		} else {
			cp.Dimensions = append(cp.Dimensions, ds)
		}
	}
	if cp.Timeout == 0 {
		cp.Timeout = DefaultCommandTimeout
	}
	if cp.Interval == 0 {
		cp.Interval = DefaultInterval
	}
	return cp, nil
}

func LoadConfig(path string) (*Config, error) {
	c := &Config{
		CheckPlugins:            make(map[string]*CheckPlugin),
		CloudWatchMetricPlugins: make(map[string]*CloudWatchMetricPlugin),
		MackerelMetricPlugins:   make(map[string]*MackerelMetricPlugin),
	}

	if err := config.LoadWithEnvTOML(&c, path); err != nil {
		return nil, err
	}

	for key, value := range c.Plugin {
		switch key {
		case "metrics":
			for id, pc := range value {
				if pc.Destination == "mackerel" {
					d, err := pc.NewMackerelDriver(id)
					if err != nil {
						return nil, err
					}
					c.MackerelMetricPlugins[id] = d
				} else {
					d, err := pc.NewCloudWatchDriver(id)
					if err != nil {
						return nil, err
					}
					c.CloudWatchMetricPlugins[id] = d
				}
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

	return c, nil
}
