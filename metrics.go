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
	shellwords "github.com/mattn/go-shellwords"
	"github.com/pkg/errors"
)

type MetricPlugin struct {
	ID         string
	Command    string
	Timeout    time.Duration
	Dimensions [][]*cloudwatch.Dimension
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

func parseMetricLine(b string) (*Metric, error) {
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

func (mp *MetricPlugin) Run(ctx context.Context, ch chan *cloudwatch.PutMetricDataInput) {
	ticker := time.NewTicker(time.Minute)
	log.Printf("[%s] starting", mp.ID)
	for {
		metrics, err := mp.Execute(ctx)
		if err != nil {
			log.Printf("[%s] %s", mp.ID, err)
		}
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
			ch <- &cloudwatch.PutMetricDataInput{
				Namespace:  aws.String(ns),
				MetricData: data,
			}
		}

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
		m, err := parseMetricLine(scanner.Text())
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
