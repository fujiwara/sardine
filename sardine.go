package sardine

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	mackerel "github.com/mackerelio/mackerel-client-go"
)

var (
	Debug                 = false
	DefaultInterval       = time.Minute
	DefaultCommandTimeout = time.Minute

	maxMetricDatum = 20
)

func Run(configPath string) error {
	cch := make(chan *cloudwatch.PutMetricDataInput, 1000)
	mch := make(chan ServiceMetric, 1000)
	conf, err := LoadConfig(configPath)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)

	go putToCloudWatch(ctx, cch)
	go putToMackerel(ctx, mch)

	for _, _mp := range conf.MetricPlugins {
		switch mp := _mp.(type) {
		case *CloudWatchMetricPlugin:
			mp.Ch = cch
			go runMetricPlugin(ctx, mp)
		case *MackerelMetricPlugin:
			mp.Ch = mch
			go runMetricPlugin(ctx, mp)
		}
		time.Sleep(time.Second)
	}
	for _, cp := range conf.CheckPlugins {
		go cp.Run(ctx, cch)
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
				b, _ := json.Marshal(in)
				log.Printf("[debug] putToCloudWatch: %s", b)
			}
			_, err := svc.PutMetricDataWithContext(ctx, in, request.WithResponseReadTimeout(30*time.Second))
			if err != nil {
				log.Println("[warn] PutMetricData to CloudWatch failed:", err)
			}
		}
	}
}

func putToMackerel(ctx context.Context, ch chan ServiceMetric) {
	c := mackerel.NewClient(os.Getenv("MACKEREL_APIKEY"))

	for {
		select {
		case <-ctx.Done():
			return
		case in := <-ch:
			if Debug {
				b, _ := json.Marshal(in)
				log.Printf("[debug] putToMackerel: %s", b)
			}
			err := c.PostServiceMetricValues(in.Service, in.MetricValues)
			if err != nil {
				log.Println("[warn] PostServiceMetricValues to Mackerel failed:", err)
			}
		}
	}
}
