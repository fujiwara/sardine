package sardine

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	"github.com/mackerelio/mackerel-agent/config"
	"github.com/mackerelio/mackerel-agent/metrics"
	"github.com/mackerelio/mackerel-client-go"
	shellwords "github.com/mattn/go-shellwords"
	"github.com/pkg/errors"
)

const (
	PluginPrefix = "custom."
)

type MetricPlugin struct {
	ID           string
	Command      string
	Timeout      time.Duration
	Interval     time.Duration
	Dimensions   [][]*cloudwatch.Dimension
	PluginDriver PluginDriver
}

type Metric struct {
	Namespace string
	Name      string
	Value     float64
	Timestamp time.Time
}

func (m *Metric) NewMetricDatum(ds []*cloudwatch.Dimension) *cloudwatch.MetricDatum {
	return &cloudwatch.MetricDatum{
		MetricName: &m.Name,
		Value:      &m.Value,
		Timestamp:  &m.Timestamp,
		Dimensions: ds,
	}
}

func (m *Metric) NewMackerelMetric(hostID string) *mackerel.HostMetricValue {
	return &mackerel.HostMetricValue{
		HostID: hostID,
		MetricValue: &mackerel.MetricValue{
			Name:  m.Name,
			Time:  m.Timestamp.Unix(),
			Value: m.Value,
		},
	}
}

type PluginDriver interface {
	enqueue([]*Metric)
	parseMetricLine(string) (*Metric, error)
}

type CloudWatchDriver struct {
	Ch         chan *cloudwatch.PutMetricDataInput
	Dimensions [][]*cloudwatch.Dimension
}

func (cd *CloudWatchDriver) enqueue(metrics []*Metric) {
	mds := make(map[string][]*cloudwatch.MetricDatum, len(cd.Dimensions)+1)
	for _, metric := range metrics {
		ns := metric.Namespace
		for _, ds := range cd.Dimensions {
			mds[ns] = append(mds[ns], metric.NewMetricDatum(ds))
		}
		// no dimension metric
		mds[ns] = append(mds[ns], metric.NewMetricDatum(nil))
	}
	for ns, data := range mds {
		cd.Ch <- &cloudwatch.PutMetricDataInput{
			Namespace:  aws.String(ns),
			MetricData: data,
		}
	}
}

func (cd *CloudWatchDriver) parseMetricLine(b string) (*Metric, error) {
	cols := strings.SplitN(b, "\t", 3)
	if len(cols) < 3 {
		return nil, errors.New("invalid metric format. insufficient columns")
	}
	name, value, timestamp := cols[0], cols[1], cols[2]
	var m Metric

	ns := strings.SplitN(name, ".", 3)
	if len(ns) != 3 {
		return nil, fmt.Errorf("invalid metric name: %s", name)
	} else {
		m.Namespace = ns[0] + "/" + ns[1]
		m.Name = ns[2]
	}

	if v, err := strconv.ParseFloat(value, 64); err != nil {
		return nil, fmt.Errorf("invalid metric value: %s", value)
	} else {
		m.Value = v
	}

	if ts, err := strconv.ParseInt(timestamp, 10, 64); err != nil {
		return nil, fmt.Errorf("invalid metric time: %s", timestamp)
	} else {
		m.Timestamp = time.Unix(ts, 0)
	}

	return &m, nil
}

type MackerelDriver struct {
	Ch     chan []*mackerel.HostMetricValue
	HostID string
}

func (md *MackerelDriver) enqueue(metrics []*Metric) {
	ms := []*mackerel.HostMetricValue{}
	for _, metric := range metrics {
		m := metric.NewMackerelMetric(me.HostID)
		ms = append(ms, m)
	}
	me.Ch <- ms
}

func (md *MackerelDriver) parseMetricLine(b string) (*Metric, error) {
	cols := strings.SplitN(b, "\t", 3)
	if len(cols) < 3 {
		return nil, errors.New("invalid metric format. insufficient columns")
	}

	name, value, timestamp := cols[0], cols[1], cols[2]
	var m Metric
	m.Name = PluginPrefix + name

	if v, err := strconv.ParseFloat(value, 64); err != nil {
		return nil, fmt.Errorf("invalid metric value: %s", value)
	} else {
		m.Value = v
	}

	if ts, err := strconv.ParseInt(timestamp, 10, 64); err != nil {
		return nil, fmt.Errorf("invalid metric time: %s", timestamp)
	} else {
		m.Timestamp = time.Unix(ts, 0)
	}

	return &m, nil
}

func (mp *MetricPlugin) Run(ctx context.Context) {
	ticker := time.NewTicker(mp.Interval)
	log.Printf("[%s] starting", mp.ID)
	for {
		metrics, err := mp.Execute(ctx)
		if err != nil {
			log.Printf("[%s] %s", mp.ID, err)
		}

		mp.PluginDriver.enqueue(metrics)

		select {
		case <-ticker.C:
			continue
		case <-ctx.Done():
			return
		}
	}
}

func (mp *MetricPlugin) Execute(_ctx context.Context) ([]*Metric, error) {
	var (
		err     error
		metrics []*Metric
	)

	ctx, cancel := context.WithTimeout(_ctx, mp.Timeout)
	defer cancel()

	args, err := shellwords.Parse(mp.Command)
	if err != nil {
		return nil, errors.Wrap(err, "parse command failed")
	}

	cmd := exec.CommandContext(ctx, args[0], args[1:]...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, errors.Wrap(err, "stdout open failed")
	}
	scanner := bufio.NewScanner(stdout)

	if err := cmd.Start(); err != nil {
		return nil, errors.Wrap(err, "command execute failed1")
	}

	for scanner.Scan() {
		m, err := mp.PluginDriver.parseMetricLine(scanner.Text())
		if err != nil {
			log.Println(err)
			continue
		}
		metrics = append(metrics, m)
	}

	err = cmd.Wait()
	if e, ok := err.(*exec.ExitError); ok {
		return nil, errors.Wrap(e, "command execute failed")
	}

	return metrics, err
}

func (mp *MetricPlugin) GraphDef() (interface{}, error) {
	cmd, err := shellwords.Parse(mp.Command)
	if err != nil {
		return nil, errors.Wrap(err, "command parse failed")
	}

	pc := &config.MetricPlugin{
		Command: config.Command{
			Args: cmd,
		},
	}
	payload, err := metrics.NewPluginGenerator(pc).PrepareGraphDefs()
	if err != nil {
		return nil, errors.Wrap(err, "parse graph defs failed")
	}

	return payload, nil
}
