package main

import (
	"context"
	"dockyard/config"
	"dockyard/pkg/kube"
	"dockyard/pkg/ui"
	"dockyard/utils"
	"fmt"
	"strings"

	// "log"
	"os"
	"os/signal"

	log "github.com/sirupsen/logrus"
)

type MyJSONFormatter struct {
}

func (f *MyJSONFormatter) Format(entry *log.Entry) ([]byte, error) {
	renderedString := fmt.Sprintf(
		"%v %v %v\n",
		entry.Time.Format("2006-01-02T15:04:05-0700"),
		strings.ToUpper(entry.Level.String()),
		entry.Message,
	)

	return []byte(renderedString), nil
}

func main() {

	config, err := config.LoadConfig()
	if err != nil {
		log.Fatal(err)
	}

	if config.Logging != nil {
		level, _ := log.ParseLevel(*config.Logging.Level)
		utils.SetupLogger(level)
	} else {
		utils.SetupLogger(log.InfoLevel)
	}

	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	c := make(chan os.Signal, 1)

	signal.Notify(c, os.Interrupt)

	defer func() {
		signal.Stop(c)
		cancel()
	}()

	renderUi(ctx, config)
}

// Renders Terminal UI for dockyard
func renderUi(ctx context.Context, config config.Config) {
	var k8sClient kube.KubeClient
	k8sClient, err := kube.NewKubeClient(config.AsgRollout.PrivateRegistry, config.AsgRollout.IgnoreNotFound, config.AsgRollout.EksClusterName)

	if err != nil {
		log.Fatal(err)
	}

	dockyardTUI := ui.NewTUI(ctx, k8sClient, config.AwsConfig, config.AsgRollout)
	dockyardTUI.EnableEventCapture()

	// Start the application.
	if err := dockyardTUI.App.SetRoot(dockyardTUI.RenderTUI(), true).EnableMouse(true).Run(); err != nil {
		panic(err)
	}

}
