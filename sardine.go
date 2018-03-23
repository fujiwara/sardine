package sardine

import (
	"context"
	"log"
	"os"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	"github.com/k0kubun/pp"
	mackerel "github.com/mackerelio/mackerel-client-go"
)

var (
	Debug                 = false
	DefaultInterval       = time.Minute
	DefaultCommandTimeout = time.Minute
)

func Run(configPath string) error {
	ch := make(chan *cloudwatch.PutMetricDataInput, 1000)
	mch := make(chan ServiceMetric, 1000)
	conf, err := LoadConfig(configPath)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)

	go putToCloudWatch(ctx, ch)
	go putToMackerel(ctx, conf, mch)

	for _, cmp := range conf.CloudWatchMetricPlugins {
		go cmp.Run(ctx, ch)
	}
	for _, mmp := range conf.MackerelMetricPlugins {
		go mmp.Run(ctx, mch)
	}
	for _, cp := range conf.CheckPlugins {
		go cp.Run(ctx, ch)
		time.Sleep(time.Second)
	}

	wg.Wait()
	return nil
}

func putToCloudWatch(ctx context.Context, ch chan *cloudwatch.PutMetricDataInput) {
	sess := session.Must(session.NewSession())
	svc := cloudwatch.New(sess)

	for {
		select {
		case <-ctx.Done():
			return
		case in := <-ch:
			if Debug {
				log.Println("putToCloudWatch", in)
			}
			_, err := svc.PutMetricDataWithContext(ctx, in, request.WithResponseReadTimeout(30*time.Second))
			if err != nil {
				log.Println("putMetricData failed:", err)
			}
		}
	}
}

func putToMackerel(ctx context.Context, conf *Config, ch chan ServiceMetric) {
	c := mackerel.NewClient(os.Getenv("MACKEREL_API_KEY"))

	for {
		select {
		case <-ctx.Done():
			return
		case in := <-ch:
			if Debug {
				log.Println("putToMackerel")
				pp.Println(in)
			}
			err := c.PostServiceMetricValues(in.Service, in.MetricValues)
			if err != nil {
				log.Println("putToMackerel failed:", err)
			}
		}
	}
}
