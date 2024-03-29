package sardine

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	config "github.com/kayac/go-config"
	shellwords "github.com/mattn/go-shellwords"
)

type Config struct {
	Plugin        map[string]map[string]*PluginConfig
	CheckPlugins  map[string]*CheckPlugin
	MetricPlugins map[string]MetricPlugin
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

func (d *Dimension) CloudWatchDimensions() ([]types.Dimension, error) {
	var ds []types.Dimension
	for _, df := range strings.Split(string(*d), ",") {
		cols := strings.SplitN(df, "=", 2)
		if len(cols) != 2 {
			return nil, fmt.Errorf("invalid dimension: %s", df)
		}
		ds = append(ds,
			types.Dimension{
				Name:  aws.String(cols[0]),
				Value: aws.String(cols[1]),
			},
		)
	}
	return ds, nil
}

func (pc *PluginConfig) NewCloudWatchMetricPlugin(id string) (*CloudWatchMetricPlugin, error) {
	if pc.Command == "" {
		return nil, fmt.Errorf("command required")
	}
	args, err := shellwords.Parse(pc.Command)
	if err != nil {
		return nil, fmt.Errorf("parse command failed: %w", err)
	}
	dimensions := [][]types.Dimension{}
	for _, d := range pc.Dimensions {
		if ds, err := d.CloudWatchDimensions(); err != nil {
			return nil, err
		} else {
			dimensions = append(dimensions, ds)
		}
	}
	mp := &CloudWatchMetricPlugin{
		id:         fmt.Sprintf("plugin.metrics.%s", id),
		command:    args,
		timeout:    pc.Timeout.Duration,
		interval:   pc.Interval.Duration,
		Dimensions: dimensions,
	}
	if mp.timeout == 0 {
		mp.timeout = DefaultCommandTimeout
	}
	if mp.interval == 0 {
		mp.interval = DefaultInterval
	}
	return mp, nil
}

func (pc *PluginConfig) NewMackerelMetricPlugin(id string) (*MackerelMetricPlugin, error) {
	if pc.Command == "" {
		return nil, fmt.Errorf("command required")
	}
	if pc.Service == "" {
		return nil, fmt.Errorf("service required")
	}
	args, err := shellwords.Parse(pc.Command)
	if err != nil {
		return nil, fmt.Errorf("parse command failed: %w", err)
	}
	mp := &MackerelMetricPlugin{
		id:       fmt.Sprintf("plugin.servicemetrics.%s", id),
		command:  args,
		timeout:  pc.Timeout.Duration,
		interval: pc.Interval.Duration,
		Service:  pc.Service,
	}
	if mp.timeout == 0 {
		mp.timeout = DefaultCommandTimeout
	}
	if mp.interval == 0 {
		mp.interval = DefaultInterval
	}
	return mp, nil
}

func (pc *PluginConfig) NewCheckPlugin(id string) (*CheckPlugin, error) {
	if pc.Namespace == "" {
		return nil, fmt.Errorf("namespace required")
	}
	if pc.Command == "" {
		return nil, fmt.Errorf("command required")
	}
	args, err := shellwords.Parse(pc.Command)
	if err != nil {
		return nil, fmt.Errorf("parse command failed: %w", err)
	}
	cp := &CheckPlugin{
		ID:        fmt.Sprintf("plugin.check.%s", id),
		Namespace: pc.Namespace,
		Command:   args,
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

func LoadConfig(ctx context.Context, path string) (*Config, error) {
	c := &Config{
		CheckPlugins:  make(map[string]*CheckPlugin),
		MetricPlugins: make(map[string]MetricPlugin),
	}
	configBytes, err := loadURL(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}
	if err := config.LoadWithEnvTOMLBytes(c, configBytes); err != nil {
		return nil, err
	}

	for key, value := range c.Plugin {
		switch key {
		case "metrics":
			for id, pc := range value {
				switch strings.ToLower(pc.Destination) {
				case "mackerel":
					d, err := pc.NewMackerelMetricPlugin(id)
					if err != nil {
						return nil, err
					}
					c.MetricPlugins[id] = d
				case "cloudwatch", "":
					d, err := pc.NewCloudWatchMetricPlugin(id)
					if err != nil {
						return nil, err
					}
					c.MetricPlugins[id] = d
				default:
					return nil, fmt.Errorf("destination %s is not allowed. use cloudwatch or mackerel", pc.Destination)
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

func loadURL(ctx context.Context, p string) ([]byte, error) {
	u, err := url.Parse(p)
	if err != nil {
		return nil, err
	}
	switch u.Scheme {
	case "http", "https":
		return fetchHTTP(ctx, u)
	case "s3":
		return fetchS3(ctx, u)
	case "file", "":
		return os.ReadFile(u.Path)
	default:
		return nil, fmt.Errorf("unsupported scheme %s", u.Scheme)
	}
}

func fetchHTTP(ctx context.Context, u *url.URL) ([]byte, error) {
	log.Println("fetching HTTP", u)
	req, _ := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func fetchS3(ctx context.Context, u *url.URL) ([]byte, error) {
	bucket, key := u.Host, strings.TrimPrefix(u.Path, "/")
	log.Printf("fetching S3 bucket=%s key=%s", bucket, key)
	region := os.Getenv("AWS_REGION")
	awscfg, err := awsConfig.LoadDefaultConfig(ctx, awsConfig.WithRegion(region))
	if err != nil {
		panic(fmt.Errorf("failed to load aws config: %w", err))
	}
	svc := s3.NewFromConfig(awscfg)
	out, err := svc.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get s3 object: %w", err)
	}
	defer out.Body.Close()
	return io.ReadAll(out.Body)
}
