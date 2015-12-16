package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/warebot/prometheusproxy/version"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

var configFile = flag.String("config.file", "promproxy.yml", "proxy config flie")

const acceptHeader = `application/vnd.google.protobuf;proto=io.prometheus.client.MetricFamily;encoding=delimited;q=0.7,text/plain;version=0.0.4;q=0.3,application/json;schema="prometheus/telemetry";version=0.0.2;q=0.2,*/*;q=0.1`

func trapSignal(ch chan os.Signal) {
	signalType := <-ch

	Warning.Println(fmt.Sprintf("Caught [%v]", signalType))
	Warning.Println("Shutting down")

	signal.Stop(ch)
	os.Exit(0)
}

func infoHandler(w http.ResponseWriter, req *http.Request) {
	encoder := json.NewEncoder(w)
	encoder.Encode(version.Map)
}

func main() {
	Info.Println("Initializing service")
	Info.Println("Version =>", version.Version)
	Info.Println("Revision =>", version.Revision)
	Info.Println("Build date =>", version.BuildDate)

	flag.Parse()

	cfg, err := readConfig(*configFile)

	if err != nil {
		panic(err.Error())
	}

	ch := make(chan os.Signal, 1)
	go trapSignal(ch)
	signal.Notify(ch, os.Interrupt, os.Kill, syscall.SIGTERM)

	client := ScrapeClient{config: cfg}
	handler := &PromProxy{client: client}
	http.Handle("/metrics", handler)
	http.HandleFunc("/", infoHandler)
	Info.Println("Starting proxy service on port", cfg.Port)
	if err = http.ListenAndServe(":"+cfg.Port, nil); err != nil {
		Error.Fatalf("Failed to start the proxy service: %v", err.Error())
	}

}
