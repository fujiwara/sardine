package sardine

import (
	"context"
	"log"
	"os/exec"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	"github.com/pkg/errors"
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

func (cp *CheckPlugin) Execute(_ctx context.Context) (CheckResult, error) {
	var err error

	ctx, cancel := context.WithTimeout(_ctx, cp.Timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, cp.Command[0], cp.Command[1:]...)
	stdoutStderr, err := cmd.CombinedOutput()
	if err == nil {
		return CheckOK, nil
	}

	// exit != 0
	if len(stdoutStderr) > 0 {
		log.Printf("[%s] %s", cp.ID, stdoutStderr)
	}
	if e2, ok := err.(*exec.ExitError); ok {
		if s, ok := e2.Sys().(syscall.WaitStatus); ok {
			if st := s.ExitStatus(); st == int(CheckFailed) || st == int(CheckWarning) {
				return CheckResult(st), err
			} else {
				return CheckUnknown, errors.Wrap(err, "command execute failed")
			}
		} else {
			panic(errors.New("unimplemented for system where exec.ExitError.Sys() is not syscall.WaitStatus."))
		}
	}
	return CheckUnknown, errors.Wrap(err, "command execute failed")
}
