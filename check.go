package sardine

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/Songmu/timeout"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

type CheckPlugin struct {
	ID         string
	Namespace  string
	Command    []string
	Timeout    time.Duration
	Interval   time.Duration
	Dimensions [][]types.Dimension
}

//go:generate stringer -type CheckResult
type CheckResult int

const (
	CheckOK CheckResult = iota
	CheckFailed
	CheckWarning
	CheckUnknown
)

func (r CheckResult) NewMetricDatum(ds []types.Dimension, ts time.Time) types.MetricDatum {
	return types.MetricDatum{
		MetricName: aws.String(r.String()),
		Value:      aws.Float64(1),
		Timestamp:  &ts,
		Dimensions: ds,
	}
}

func (cp *CheckPlugin) Run(ctx context.Context, wg *sync.WaitGroup, ch chan *cloudwatch.PutMetricDataInput) {
	defer wg.Done()
	ticker := time.NewTicker(cp.Interval)
	log.Printf("[%s] starting", cp.ID)
	for {
		if err := cp.RunAtOnce(ctx, ch); err != nil {
			log.Println(err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			continue
		}
	}
}

func (cp *CheckPlugin) RunAtOnce(ctx context.Context, ch chan *cloudwatch.PutMetricDataInput) error {
	res, err := cp.Execute(ctx)
	if err != nil {
		return fmt.Errorf("[%s] %s %w", cp.ID, res, err)
	}
	now := time.Now()
	md := make([]types.MetricDatum, 0, len(cp.Dimensions)+1)
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
	return nil
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
