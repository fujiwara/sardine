package sardine

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
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

	wg := new(sync.WaitGroup)
	wg.Add(2)
	go putToCloudWatch(ctx, wg, cch)
	go putToMackerel(ctx, wg, mch)

	for _, _mp := range conf.MetricPlugins {
		switch mp := _mp.(type) {
		case *CloudWatchMetricPlugin:
			mp.Ch = cch
			wg.Add(1)
			go runMetricPlugin(ctx, wg, mp)
		case *MackerelMetricPlugin:
			mp.Ch = mch
			wg.Add(1)
			go runMetricPlugin(ctx, wg, mp)
		}
		time.Sleep(time.Second)
	}
	for _, cp := range conf.CheckPlugins {
		wg.Add(1)
		go cp.Run(ctx, wg, cch)
		time.Sleep(time.Second)
	}

	<-ctx.Done()
	log.Println("shutting down. waiting for complete...")
	close(cch)
	close(mch)
	wg.Wait()
	log.Println("shutdown complete")
	return nil
}

func RunAtOnce(ctx context.Context, configPath string) error {
	cch := make(chan *cloudwatch.PutMetricDataInput, 10000)
	mch := make(chan ServiceMetric, 10000)
	conf, err := LoadConfig(ctx, configPath)
	if err != nil {
		return err
	}

	wg := new(sync.WaitGroup)
	for _, _mp := range conf.MetricPlugins {
		switch mp := _mp.(type) {
		case *CloudWatchMetricPlugin:
			mp.Ch = cch
			log.Printf("[%s] run", mp.ID())
			runMetricPluginAtOnce(ctx, mp)
		case *MackerelMetricPlugin:
			mp.Ch = mch
			log.Printf("[%s] run", mp.ID())
			runMetricPluginAtOnce(ctx, mp)
		}
	}
	for _, cp := range conf.CheckPlugins {
		log.Printf("[%s] run", cp.ID)
		cp.RunAtOnce(ctx, cch)
	}
	close(cch)
	close(mch)
	wg.Add(2)
	putToCloudWatch(ctx, wg, cch)
	putToMackerel(ctx, wg, mch)

	log.Println("shutting down. waiting for complete...")
	wg.Wait()
	log.Println("shutdown complete")
	return nil
}

func putToCloudWatch(ctx context.Context, wg *sync.WaitGroup, ch chan *cloudwatch.PutMetricDataInput) {
	defer wg.Done()
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
		case in, ok := <-ch:
			if !ok {
				log.Println("putToCloudWatch: channel closed")
				return
			}
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

func putToMackerel(ctx context.Context, wg *sync.WaitGroup, ch chan ServiceMetric) {
	defer wg.Done()
	c := mackerel.NewClient(os.Getenv("MACKEREL_APIKEY"))

	for {
		select {
		case <-ctx.Done():
			return
		case in, ok := <-ch:
			if !ok {
				log.Println("putToMackerel: channel closed")
				return
			}
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
