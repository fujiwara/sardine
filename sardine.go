package sardine

import (
	"context"
	"log"
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
	conf, err := LoadConfig(configPath)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)

	ch := make(chan *cloudwatch.PutMetricDataInput, 1000)
	mch := make(chan []*mackerel.HostMetricValue, 1000)
	go putToCloudWatch(ctx, ch)
	go putToMackerel(ctx, conf, mch)

	for _, mp := range conf.MetricPlugins {
		go mp.Run(ctx, ch)
		time.Sleep(time.Second)
	}
	for _, cp := range conf.CheckPlugins {
		go cp.Run(ctx, ch)
		time.Sleep(time.Second)
	}
	for _, mp := range conf.MackerelMetricPlugins {
		go mp.RunForMackerel(ctx, mch)
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
				log.Println("put", in)
			}
			_, err := svc.PutMetricDataWithContext(ctx, in, request.WithResponseReadTimeout(30*time.Second))
			if err != nil {
				log.Println("putMetricData failed:", err)
			}
		}
	}
}

func putToMackerel(ctx context.Context, c *Config, ch chan []*mackerel.HostMetricValue) {
	client := mackerel.NewClient(c.APIKey)

	for {
		select {
		case <-ctx.Done():
			return
		case in := <-ch:
			if Debug {
				log.Println("put")
				pp.Println(in)
			}
			err := client.PostHostMetricValues(in)
			if err != nil {
				log.Println("putToMackerel failed:", err)
			}
		}
	}
}
