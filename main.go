package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	tea "github.com/charmbracelet/bubbletea"
)

type lambdaDetailMsg struct {
	info lambdaInfo
}

type logStreamMsg struct {
	items []logStream
}

// A log event message is being sent to the model containing
// log stream messages in the format of
//
//	{
//		{event timestamp, message},
//	}
type logEventMsg struct {
	events [][]string
}

type errMsg struct {
	err error
}

type lambdaItem struct {
	name     string
	logGroup string
}

func (l lambdaItem) Title() string {
	return l.name
}

func (l lambdaItem) Description() string {
	return l.logGroup
}

func (l lambdaItem) FilterValue() string {
	return l.name
}

type logStreamItem struct {
	name               string
	lastEventTimestamp string
}

func (l logStreamItem) Title() string {
	return l.name
}

func (l logStreamItem) Description() string {
	return l.lastEventTimestamp
}

func (l logStreamItem) FilterValue() string {
	return l.name
}

func main() {
	f, err := tea.LogToFile("debug.log", "debug")
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	err = run()
	if err != nil {
		log.Println(err)
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		return err
	}

	credentials, err := cfg.Credentials.Retrieve(context.Background())

	lambdaClient := lambda.NewFromConfig(cfg)
	cloudwatchClient := cloudwatchlogs.NewFromConfig(cfg)
	items, err := getLambdaFunctions(context.Background(), lambdaClient)
	if err != nil {
		return err
	}

	reqCh := make(chan interface{})
	model := newModel(reqCh, credentials.AccountID, items)

	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	go handleRequests(p, cloudwatchClient, lambdaClient, reqCh)

	if _, err := p.Run(); err != nil {
		return err
	}

	return nil
}

func handleRequests(p *tea.Program, cwClient *cloudwatchlogs.Client, lambdaClient *lambda.Client, reqCh <-chan interface{}) {
	for {
		msg := <-reqCh
		switch msg := msg.(type) {
		case logStreamReq:
			streams, err := getLogStreams(context.Background(), cwClient, msg.logGroup)
			if err != nil {
				log.Printf("[Error] %v", err)
				p.Send(errMsg{err})
				continue
			}

			p.Send(logStreamMsg{items: streams})

		case logEventReq:
			events, err := getLogEvents(context.Background(), cwClient, msg.logGroup, msg.logStream)
			if err != nil {
				log.Printf("[Error] %v", err)
				p.Send(errMsg{err})

				continue
			}

			p.Send(logEventMsg{events: events})

		case lambdaDetailReq:
			lambdaInfo, err := getLambdaInfo(context.Background(), lambdaClient, msg.name)
			if err != nil {
				log.Printf("[Error] %v", err)
				p.Send(errMsg{err})

				continue
			}
			p.Send(lambdaDetailMsg{info: lambdaInfo})
		}
	}
}
