package sardine

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
)

var (
	Debug                 = false
	DefaultInterval       = time.Minute
	DefaultCommandTimeout = time.Minute
)

func Run(configPath string) error {
	ch := make(chan *cloudwatch.PutMetricDataInput, 1000)
	conf, err := LoadConfig(configPath, ch)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)

	go putToCloudWatch(ctx, ch)

	for _, mp := range conf.MetricPlugins {
		go mp.Run(ctx)
		time.Sleep(time.Second)
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
				log.Println("put", in)
			}
			_, err := svc.PutMetricDataWithContext(ctx, in, request.WithResponseReadTimeout(30*time.Second))
			if err != nil {
				log.Println("putMetricData failed:", err)
			}
		}
	}
}
