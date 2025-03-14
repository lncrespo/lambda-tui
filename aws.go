package main

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/charmbracelet/bubbles/list"
)

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

func getLogStreams(ctx context.Context, c *cloudwatchlogs.Client, logGroup string) ([][]string, error) {
	res, err := c.DescribeLogStreams(ctx, &cloudwatchlogs.DescribeLogStreamsInput{
		LogGroupName: &logGroup,
		Descending:   ptr(true),
	})
	if err != nil {
		return nil, err
	}

	streams := make([][]string, 0, len(res.LogStreams))
	requests := 1
	maxRequests := 10

	for {
		for _, stream := range res.LogStreams {
			streams = append(streams, []string{
				*stream.LogStreamName,
				time.Unix(*stream.LastEventTimestamp/1000, 0).Format(time.RFC1123),
			})
		}

		if res.NextToken == nil || requests == maxRequests {
			break
		}

		res, err = c.DescribeLogStreams(ctx, &cloudwatchlogs.DescribeLogStreamsInput{
			LogGroupName: &logGroup,
			Descending:   ptr(true),
			NextToken:    res.NextToken,
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
				*event.Message,
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
