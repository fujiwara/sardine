package sardine

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"syscall"
	"time"

	"github.com/Songmu/timeout"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
)

type CheckPlugin struct {
	ID         string
	Namespace  string
	Command    []string
	Timeout    time.Duration
	Interval   time.Duration
	Dimensions [][]*cloudwatch.Dimension
}

//go:generate stringer -type CheckResult
type CheckResult int

const (
	CheckOK CheckResult = iota
	CheckFailed
	CheckWarning
	CheckUnknown
)

func (r CheckResult) NewMetricDatum(ds []*cloudwatch.Dimension, ts time.Time) *cloudwatch.MetricDatum {
	return &cloudwatch.MetricDatum{
		MetricName: aws.String(r.String()),
		Value:      aws.Float64(1),
		Timestamp:  &ts,
		Dimensions: ds,
	}
}

func (cp *CheckPlugin) Run(ctx context.Context, ch chan *cloudwatch.PutMetricDataInput) {
	ticker := time.NewTicker(cp.Interval)
	log.Printf("[%s] starting", cp.ID)
	now := time.Now()
	for {
		res, err := cp.Execute(ctx)
		if err != nil {
			log.Printf("[%s] %s %s", cp.ID, res, err)
		}

		md := make([]*cloudwatch.MetricDatum, 0, len(cp.Dimensions)+1)
		for _, ds := range cp.Dimensions {
			md = append(md, res.NewMetricDatum(ds, now))
		}
		// no dimension metric
		md = append(md, res.NewMetricDatum(nil, now))

		// split MetricDatum/20
		for i := 0; i <= len(md)/maxMetricDatum; i++ {
			first := i * maxMetricDatum
			last := first + maxMetricDatum
			if last > len(md) {
				last = len(md)
			}
			if len(md[first:last]) == 0 {
				break
			}
			ch <- &cloudwatch.PutMetricDataInput{
				Namespace:  aws.String(cp.Namespace),
				MetricData: md[first:last],
			}
		}

		select {
		case now = <-ticker.C:
			continue
		case <-ctx.Done():
			return
		}
	}
}

func (cp *CheckPlugin) Execute(ctx context.Context) (CheckResult, error) {
	tio := &timeout.Timeout{
		Duration:  cp.Timeout,
		KillAfter: 5 * time.Second,
		Signal:    syscall.SIGTERM,
		Cmd:       exec.Command(cp.Command[0], cp.Command[1:]...),
	}
	status, stdout, stderr, err := tio.Run()
	if len(stdout) > 0 {
		log.Printf("[%s] %s", cp.ID, stdout)
	}
	if len(stderr) > 0 {
		log.Printf("[%s] %s", cp.ID, stderr)
	}
	if status.IsTimedOut() || status.IsKilled() {
		return CheckUnknown, fmt.Errorf("command execute timed out")
	}
	if err != nil {
		return CheckUnknown, fmt.Errorf("command execute failed: %w", err)
	}

	st := status.GetExitCode()
	switch st {
	case 0:
		return CheckOK, nil
	case int(CheckFailed), int(CheckWarning):
		return CheckResult(st), err
	default:
		return CheckUnknown, fmt.Errorf("command execute failed with exit code %d", st)
	}
}
