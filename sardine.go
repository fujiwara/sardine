package sardine

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	mackerel "github.com/mackerelio/mackerel-client-go"
)

var (
	Debug                 = false
	DefaultInterval       = time.Minute
	DefaultCommandTimeout = time.Minute

	maxMetricDatum = 20
)

func Run(ctx context.Context, configPath string) error {
	cch := make(chan *cloudwatch.PutMetricDataInput, 1000)
	mch := make(chan ServiceMetric, 1000)
	conf, err := LoadConfig(ctx, configPath)
	if err != nil {
		return err
	}

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

	<-ctx.Done()
	log.Println("shutting down")
	return nil
}

func putToCloudWatch(ctx context.Context, ch chan *cloudwatch.PutMetricDataInput) {
	region := os.Getenv("AWS_REGION")
	awscfg, err := awsConfig.LoadDefaultConfig(ctx, awsConfig.WithRegion(region))
	if err != nil {
		panic(fmt.Errorf("failed to load aws config: %w", err))
	}
	svc := cloudwatch.NewFromConfig(awscfg)

	for {
		select {
		case <-ctx.Done():
			return
		case in := <-ch:
			if Debug {
				b, _ := json.Marshal(in)
				log.Printf("putToCloudWatch: %s", b)
			}
			_, err := svc.PutMetricData(ctx, in)
			if err != nil {
				log.Println("PutMetricData to CloudWatch failed:", err)
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
			if len(in.MetricValues) == 0 {
				continue
			}
			if Debug {
				b, _ := json.Marshal(in)
				log.Printf("putToMackerel: %s", b)
			}
			err := c.PostServiceMetricValues(in.Service, in.MetricValues)
			if err != nil {
				log.Println("PostServiceMetricValues to Mackerel failed:", err)
			}
		}
	}
}
