package sardine

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	mackerel "github.com/mackerelio/mackerel-client-go"
	"github.com/pkg/errors"
)

type MetricPlugin interface {
	ID() string
	Command() []string
	Timeout() time.Duration
	Interval() time.Duration
	Enqueue([]*Metric)
	ParseMetricLine(string) (*Metric, error)
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

type ServiceMetric struct {
	Service      string
	MetricValues []*mackerel.MetricValue
}

type CloudWatchMetricPlugin struct {
	id         string
	command    []string
	timeout    time.Duration
	interval   time.Duration
	Dimensions [][]*cloudwatch.Dimension
	Ch         chan *cloudwatch.PutMetricDataInput
}

func (mp *CloudWatchMetricPlugin) ID() string {
	return mp.id
}

func (mp *CloudWatchMetricPlugin) Command() []string {
	return mp.command
}

func (mp *CloudWatchMetricPlugin) Timeout() time.Duration {
	return mp.timeout
}

func (mp *CloudWatchMetricPlugin) Interval() time.Duration {
	return mp.interval
}

func (mp *CloudWatchMetricPlugin) Enqueue(metrics []*Metric) {
	mds := make(map[string][]*cloudwatch.MetricDatum, len(mp.Dimensions)+1)
	for _, metric := range metrics {
		ns := metric.Namespace
		for _, ds := range mp.Dimensions {
			mds[ns] = append(mds[ns], metric.NewMetricDatum(ds))
		}
		// no dimension metric
		mds[ns] = append(mds[ns], metric.NewMetricDatum(nil))
	}
	for ns, data := range mds {
		mp.Ch <- &cloudwatch.PutMetricDataInput{
			Namespace:  aws.String(ns),
			MetricData: data,
		}
	}
}

func (cmp *CloudWatchMetricPlugin) ParseMetricLine(b string) (*Metric, error) {
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

type MackerelMetricPlugin struct {
	id       string
	command  []string
	timeout  time.Duration
	interval time.Duration
	Service  string
	Ch       chan ServiceMetric
}

func (mp *MackerelMetricPlugin) ID() string {
	return mp.id
}

func (mp *MackerelMetricPlugin) Command() []string {
	return mp.command
}

func (mp *MackerelMetricPlugin) Timeout() time.Duration {
	return mp.timeout
}

func (mp *MackerelMetricPlugin) Interval() time.Duration {
	return mp.interval
}

func (mp *MackerelMetricPlugin) Enqueue(metrics []*Metric) {
	mv := []*mackerel.MetricValue{}
	for _, m := range metrics {
		mv = append(mv, &mackerel.MetricValue{
			Name:  m.Name,
			Value: m.Value,
			Time:  m.Timestamp.Unix(),
		})
	}

	mp.Ch <- ServiceMetric{
		Service:      mp.Service,
		MetricValues: mv,
	}
}

func (mp *MackerelMetricPlugin) ParseMetricLine(b string) (*Metric, error) {
	cols := strings.SplitN(b, "\t", 3)
	if len(cols) < 3 {
		return nil, errors.New("invalid metric format. insufficient columns")
	}
	name, value, timestamp := cols[0], cols[1], cols[2]
	m := Metric{Name: name}

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

func runMetricPlugin(ctx context.Context, mp MetricPlugin) {
	ticker := time.NewTicker(mp.Interval())
	log.Printf("[%s] starting", mp.ID())
	for {
		metrics, err := executeCommand(ctx, mp)
		if err != nil {
			log.Printf("[%s] %s", mp.ID(), err)
		}
		mp.Enqueue(metrics)

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			continue
		}
	}
}

func executeCommand(ctx context.Context, mp MetricPlugin) ([]*Metric, error) {
	var (
		err     error
		metrics []*Metric
	)

	ctx, cancel := context.WithTimeout(ctx, mp.Timeout())
	defer cancel()

	args := mp.Command()
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, errors.Wrap(err, "stdout open failed")
	}
	scanner := bufio.NewScanner(stdout)

	if err := cmd.Start(); err != nil {
		return nil, errors.Wrap(err, "command execute failed1")
	}

	for scanner.Scan() {
		m, err := mp.ParseMetricLine(scanner.Text())
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
