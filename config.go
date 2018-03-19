package sardine

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	"github.com/k0kubun/pp"
	config "github.com/kayac/go-config"
	mc "github.com/mackerelio/mackerel-agent/config"
	mackerel "github.com/mackerelio/mackerel-client-go"
	"github.com/pkg/errors"
)

type Config struct {
	hostID               string
	APIKey               string
	Plugin               map[string]map[string]*PluginConfig
	MetricPlugins        map[string]*MetricPlugin
	CheckPlugins         map[string]*CheckPlugin
	CustomIdentifierList sync.Map
	MackerelClient       *mackerel.Client
}

func (c *Config) GetHostIDByCustomIdentifier(customIdentifier string) (string, error) {
	if ci, ok := c.CustomIdentifierList.Load(customIdentifier); ok {
		return ci.(string), nil
	}

	hosts, err := c.MackerelClient.FindHosts(&mackerel.FindHostsParam{
		CustomIdentifier: customIdentifier,
	})
	if err != nil {
		return "", errors.Wrap(err, "FindHosts failed.")
	}
	if len(hosts) == 0 {
		return "", errors.New(fmt.Sprint("No such host. custom_identifier=", customIdentifier))
	}
	if len(hosts) > 1 {
		return "", errors.New(fmt.Sprint("Ambiguous custom_identifier. Found host must be 1. custom_identifier=", customIdentifier,
			"found host=", hosts))
	}

	hostID := hosts[0].ID
	c.CustomIdentifierList.Store(customIdentifier, hostID)

	return hostID, nil
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
	Namespace        string
	Command          string
	Timeout          duration
	Interval         duration
	Dimensions       []*Dimension
	CustomIdentifier string `toml:"custom_identifier"`
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

func (pc *PluginConfig) NewMetricPlugin(id string, ch chan *cloudwatch.PutMetricDataInput) (*MetricPlugin, error) {
	if pc.Command == "" {
		return nil, errors.New("command required")
	}
	driver := CloudWatchDriver{Ch: ch}
	for _, d := range pc.Dimensions {
		if ds, err := d.CloudWatchDimensions(); err != nil {
			return nil, err
		} else {
			driver.Dimensions = append(driver.Dimensions, ds)
		}
	}
	mp := &MetricPlugin{
		ID:           fmt.Sprintf("plugin.metrics.%s", id),
		Command:      pc.Command,
		Timeout:      pc.Timeout.Duration,
		Interval:     pc.Interval.Duration,
		PluginDriver: &driver,
	}
	if mp.Timeout == 0 {
		mp.Timeout = DefaultCommandTimeout
	}
	if mp.Interval == 0 {
		mp.Interval = DefaultInterval
	}
	return mp, nil
}

func (pc *PluginConfig) NewMackerelMetricPlugin(conf *Config, id string, ch chan []*mackerel.HostMetricValue) (*MetricPlugin, error) {
	if pc.Command == "" {
		return nil, errors.New("command required")
	}
	hostID := conf.hostID
	if pc.CustomIdentifier != "" {
		var err error
		hostID, err = conf.GetHostIDByCustomIdentifier(pc.CustomIdentifier)
		if err != nil {
			return nil, errors.Wrap(err, "failed GetHostIDByCustomIdentifier")
		}
	}
	if hostID == "" {
		return nil, errors.New("no hostid.")
	}

	mp := &MetricPlugin{
		ID:           fmt.Sprintf("plugin.mackerelmetrics.%s", id),
		Command:      pc.Command,
		Timeout:      pc.Timeout.Duration,
		Interval:     pc.Interval.Duration,
		PluginDriver: &MackerelDriver{HostID: hostID, Ch: ch},
	}
	if mp.Timeout == 0 {
		mp.Timeout = DefaultCommandTimeout
	}
	if mp.Interval == 0 {
		mp.Interval = DefaultInterval
	}

	graphDef, err := mp.GraphDef()
	if err != nil {
		return nil, errors.Wrap(err, "create graphdefinition failed")
	}
	if Debug {
		pp.Println(graphDef)
	}

	r, err := conf.MackerelClient.PostJSON("/api/v0/graph-defs/create", graphDef)
	if err != nil {
		return nil, errors.Wrap(err, "post graphdefinition failed")
	}
	defer r.Body.Close()

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

func LoadConfig(path string, ch chan *cloudwatch.PutMetricDataInput, mch chan []*mackerel.HostMetricValue) (*Config, error) {
	c := &Config{
		MetricPlugins: make(map[string]*MetricPlugin),
		CheckPlugins:  make(map[string]*CheckPlugin),
	}

	if err := config.LoadWithEnvTOML(&c, path); err != nil {
		return nil, err
	}

	if c.APIKey != "" {
		hostID, err := mc.DefaultConfig.LoadHostID()
		if err != nil {
			return c, errors.Wrap(err, "failed load host id")
		}
		if hostID != "" {
			c.hostID = hostID
		}

		c.MackerelClient = mackerel.NewClient(c.APIKey)
	}

	for key, value := range c.Plugin {
		switch key {
		case "metrics":
			for id, pc := range value {
				mp, err := pc.NewMetricPlugin(id, ch)
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
		case "mackerelmetrics":
			for id, pc := range value {
				mmp, err := pc.NewMackerelMetricPlugin(c, id, mch)
				if err != nil {
					return nil, err
				}
				c.MetricPlugins[id] = mmp
			}
		default:
			return nil, fmt.Errorf("unknown config section [plugin.%s]", key)
		}
	}
	c.Plugin = nil

	return c, nil
}
