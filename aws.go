package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/charmbracelet/bubbles/list"
)

type lambdaInfo struct {
	arn              string
	name             string
	description      string
	lastModified     string
	runtime          string
	arch             string
	memory           uint32
	ephemeralStorage uint32
	timeout          uint32
	envVars          [][]string
	tags             [][]string
}

type logStream struct {
	name               string
	lastEventTimestamp string
	expired            bool
}

func getLambdaFunctions(ctx context.Context, c *lambda.Client) ([]list.Item, error) {
	res, err := c.ListFunctions(ctx, nil)
	if err != nil {
		return nil, err
	} else if res == nil {
		return nil, fmt.Errorf("received nil response")
	}

	functions := make([]list.Item, 0, len(res.Functions))

	for {
		for _, fn := range res.Functions {
			functions = append(functions, lambdaItem{name: *fn.FunctionName, logGroup: *fn.LoggingConfig.LogGroup})
		}

		if res.NextMarker == nil {
			break
		}

		res, err = c.ListFunctions(ctx, &lambda.ListFunctionsInput{Marker: res.NextMarker})
		if err != nil {
			return nil, err
		}
	}

	return functions, nil
}

func getLambdaInfo(ctx context.Context, c *lambda.Client, name string) (lambdaInfo, error) {
	res, err := c.GetFunction(ctx, &lambda.GetFunctionInput{
		FunctionName: &name,
	})
	if err != nil {
		return lambdaInfo{}, err
	}

	if res.Configuration == nil {
		return lambdaInfo{}, fmt.Errorf("received nil function config")
	}

	fnInfo := lambdaInfo{
		arn:              "invalid arn",
		name:             name,
		description:      "invalid description",
		lastModified:     "invalid last modified date",
		runtime:          "invalid runtime",
		arch:             "invalid arch",
		memory:           0,
		ephemeralStorage: 0,
		timeout:          0,
		envVars:          [][]string{},
		tags:             [][]string{},
	}

	if len(res.Configuration.Architectures) > 0 {
		fnInfo.arch = string(res.Configuration.Architectures[0])
	}

	if res.Configuration.FunctionArn != nil {
		fnInfo.arn = *res.Configuration.FunctionArn
	}

	if res.Configuration.Description != nil {
		fnInfo.description = *res.Configuration.Description
	}

	if res.Configuration.MemorySize != nil {
		fnInfo.memory = uint32(*res.Configuration.MemorySize)
	}

	if res.Configuration.EphemeralStorage != nil && res.Configuration.EphemeralStorage.Size != nil {
		fnInfo.ephemeralStorage = uint32(*res.Configuration.EphemeralStorage.Size)
	}

	if res.Configuration.LastModified != nil {
		fnInfo.lastModified = *res.Configuration.LastModified
	}

	if res.Configuration.Timeout != nil {
		fnInfo.timeout = uint32(*res.Configuration.Timeout)
	}

	fnInfo.runtime = string(res.Configuration.Runtime)

	fnInfo.tags = make([][]string, 0, len(res.Tags))
	for k, v := range res.Tags {
		fnInfo.tags = append(fnInfo.tags, []string{k, v})
	}

	if res.Configuration.Environment == nil {
		return fnInfo, nil
	}

	fnInfo.envVars = make([][]string, 0, len(res.Configuration.Environment.Variables))
	for k, v := range res.Configuration.Environment.Variables {
		fnInfo.envVars = append(fnInfo.envVars, []string{k, v})
	}

	return fnInfo, nil
}

func getLogStreams(ctx context.Context, c *cloudwatchlogs.Client, logGroup string) ([]logStream, error) {
	logGroupRes, err := c.DescribeLogGroups(ctx, &cloudwatchlogs.DescribeLogGroupsInput{
		LogGroupNamePrefix: &logGroup,
	})

	if len(logGroupRes.LogGroups) == 0 {
		return nil, fmt.Errorf("log group not found")
	}

	retention := int64(-1)
	if logGroupRes.LogGroups[0].RetentionInDays != nil {
		retention = int64(*logGroupRes.LogGroups[0].RetentionInDays) * 86400000
	}

	res, err := c.DescribeLogStreams(ctx, &cloudwatchlogs.DescribeLogStreamsInput{
		LogGroupName: &logGroup,
		Descending:   ptr(true),
		OrderBy:      types.OrderByLastEventTime,
	})
	if err != nil {
		return nil, err
	}

	streams := make([]logStream, 0, len(res.LogStreams))
	requests := 1
	maxRequests := 10

	for {
		for _, stream := range res.LogStreams {
			expired := false
			if stream.LastEventTimestamp != nil {
				expired = time.UnixMilli(*stream.LastEventTimestamp + retention).Before(time.Now())
			}

			streams = append(streams, logStream{
				name:               *stream.LogStreamName,
				lastEventTimestamp: time.Unix(*stream.LastEventTimestamp/1000, 0).Format(time.RFC1123),
				expired:            expired,
			})
		}

		if res.NextToken == nil || requests == maxRequests {
			break
		}

		res, err = c.DescribeLogStreams(ctx, &cloudwatchlogs.DescribeLogStreamsInput{
			LogGroupName: &logGroup,
			Descending:   ptr(true),
			NextToken:    res.NextToken,
			OrderBy:      types.OrderByLastEventTime,
		})
		if err != nil {
			return nil, err
		}

		requests++
	}

	return streams, nil
}

func getLogEvents(ctx context.Context, c *cloudwatchlogs.Client, logGroup string, logStream string) ([][]string, error) {
	res, err := c.GetLogEvents(ctx, &cloudwatchlogs.GetLogEventsInput{
		LogGroupName:  &logGroup,
		LogStreamName: &logStream,
	})
	if err != nil {
		return nil, err
	}

	requests := 1
	maxRequests := 5
	logEvents := make([][]string, 0, 100)

	for {
		for _, event := range res.Events {
			if event.Message == nil {
				continue
			}

			logEvents = append(logEvents, []string{
				time.Unix(*event.Timestamp/1000, 0).Format(time.RFC1123),
				strings.TrimSuffix(*event.Message, "\n"),
			})
		}

		if res.NextBackwardToken == nil || requests == maxRequests {
			break
		}

		res, err = c.GetLogEvents(ctx, &cloudwatchlogs.GetLogEventsInput{
			LogGroupName:  &logGroup,
			LogStreamName: &logStream,
			NextToken:     res.NextBackwardToken,
		})
		if err != nil {
			return nil, err
		}
		requests++
	}

	return logEvents, nil
}

func ptr[T ~string | ~float64 | ~float32 | ~int | ~uint | ~bool](val T) *T {
	return &val
}
