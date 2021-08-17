package main

import (
	"flag"
	"github.com/go-rod/rod"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"log"
	"net/http"
	"path"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type PolymerConfig struct {
	DefaultType string `yaml:"defaultType"`
	Name        string `yaml:"name"`
	Steps       []Step `yaml:"steps"`
}

type Step struct {
	Action string `yaml:"action"`
	Name   string `yaml:"name"`
	Inputs []StepInput `yaml:"inputs"`
	Options map[string]string `yaml:"options"`
}

type StepInput struct {
	Element  Element `yaml:"element"`
	Action   string `yaml:"action"`
	Value    string `yaml:"value"`
}

type Element struct {
	Identifier string `yaml:"identifier"`
}

var addr = flag.String("listen-address", ":8080", "The address to listen on for HTTP requests.")
var configFile= flag.String("config", ".polymer.yaml", "The config file to load for synthetics")
var page *rod.Page

func init() {
	prometheus.MustRegister(collectors.NewBuildInfoCollector())
}

func main() {
	flag.Parse()
	// Expose the registered metrics via HTTP.
	http.Handle("/metrics", promhttp.HandlerFor(
		prometheus.DefaultGatherer,
		promhttp.HandlerOpts{
			// Opt into OpenMetrics to support exemplars.
			EnableOpenMetrics: true,
		},
	))

	yamlFile, err := ioutil.ReadFile(*configFile)
	c := &PolymerConfig{}
	err = yaml.Unmarshal(yamlFile, c)
	if err != nil {
		log.Fatalf("Unmarshal: %v", err)
	}

	executor := &Executor{Config: c}
	http.HandleFunc(path.Join("/probe"), func(w http.ResponseWriter, r *http.Request) {
		probeHandler(w, r, executor)
	})

	log.Println("Starting server")
	log.Fatal(http.ListenAndServe(*addr, nil))
}

type Executor struct {
	Config *PolymerConfig
}

func probeHandler(w http.ResponseWriter, r *http.Request, e *Executor) {
	probeDurationGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "polymer",
		Name: "probe_duration_seconds",
		Help: "Returns how long the probe took to complete in seconds",
	})

	registry := prometheus.NewRegistry()
	registry.MustRegister(probeDurationGauge)

	log.Println("Iterating over config steps")
	browser := rod.New()

	start := time.Now()

	for _, step := range e.Config.Steps {
		log.Println(step.Name)
		durationGauge := prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "polymer",
				Name: "step_duration_seconds",
				Help: "Returns how long each step took to complete in seconds",
				ConstLabels: map[string]string{
					"step": step.Name,
				},
			},
		)
		successGauge := prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "polymer",
				Name: "step_success",
				Help: "Whether the step execution was successful",
				ConstLabels: map[string]string{
					"step": step.Name,
				},
			})
		registry.MustRegister(durationGauge)
		registry.MustRegister(successGauge)
		stepStart := time.Now()

		// Execute our step logic

		switch step.Action {
		  case "visit":
		  	log.Println("Connecting to specified URL")
			page = browser.MustConnect().MustPage(step.Options["url"])
		  case "input":
		  	for _, input := range step.Inputs {
		  		element := page.MustElement(input.Element.Identifier)
		  		switch input.Action {
				  case "click":
				  	element.MustInput(input.Value);
				  case "submit":
				    element.MustClick();
				}
			}
		}
		// End logic execution

		stepDuration := time.Since(stepStart).Seconds()
		log.Println(stepDuration)
		durationGauge.Set(stepDuration)
	}

	//success := prober(ctx, target, module, registry, sl)
	duration := time.Since(start).Seconds()
	probeDurationGauge.Set(duration)

	//if success {
	//	probeSuccessGauge.Set(1)
	//	level.Info(sl).Log("msg", "Probe succeeded", "duration_seconds", duration)
	//} else {
	//	level.Error(sl).Log("msg", "Probe failed", "duration_seconds", duration)
	//}

	//debugOutput := DebugOutput(&module, &sl.buffer, registry)
	//rh.Add(moduleName, target, debugOutput, success)
	//
	//if r.URL.Query().Get("debug") == "true" {
	//	w.Header().Set("Content-Type", "text/plain")
	//	w.Write([]byte(debugOutput))
	//	return
	//}

	h := promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
	h.ServeHTTP(w, r)
}
