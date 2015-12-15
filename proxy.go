package main

import (
	"errors"
	"fmt"
	"github.com/golang/protobuf/proto"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	_ "github.com/prometheus/common/model"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	_ "time"
)

type PromProxy struct {
	Config *Config
}

func (p *PromProxy) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	queryParams := req.URL.Query()
	serviceName := queryParams.Get("service")
	labels := queryParams.Get("labels")

	adhocLabels := make(map[string]string)

	if len(labels) > 0 {
		labelPairSet := strings.Split(labels, ",")
		for _, labelPair := range labelPairSet {
			labelKeyValue := strings.Split(labelPair, "|")
			if len(labelKeyValue) > 1 {
				adhocLabels[labelKeyValue[0]] = labelKeyValue[1]
			}
		}
	}

	samples, err := p.scrape(serviceName, adhocLabels)

	if err != nil && err != io.EOF {
		Error.Printf("%v\n", err.Error())
		w.Write([]byte(err.Error()))
		return
	}

	// Negotiates content-type based on accept headers and creates the encoder accordingly
	contentType := expfmt.Negotiate(req.Header)
	// Set the Content type header based on the negotiated accept header negotiation
	w.Header().Set("Content-type", string(contentType))
	encoder := expfmt.NewEncoder(w, contentType)
	if err != nil {
		if err != io.EOF {
			Error.Printf("%\nv", err.Error())
			w.Write([]byte(err.Error()))
			return
		}
	}

	// Sort the metrics by metric family name
	names := make([]string, 0, len(samples))
	for name := range samples {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		if err := encoder.Encode(samples[name]); err != nil {
			Error.Printf("%v\n", err.Error())
			w.Write([]byte(err.Error()))
		}
	}
}

func (p *PromProxy) getLabels(serviceName string) (map[string]string, error) {
	if len(serviceName) > 0 {
		service, ok := p.Config.Services[serviceName]
		if !ok {
			return nil, errors.New("unknown service type")
		}

		return service.Labels, nil
	}
	m := make(map[string]string)
	return m, nil
}

func (p *PromProxy) scrape(serviceName string, adhocLabels map[string]string) (map[string]*dto.MetricFamily, error) {

	service, ok := p.Config.Services[serviceName]
	if !ok {
		return nil, errors.New(fmt.Sprintf("unknown service type '%v'", serviceName))
	}

	target, err := url.Parse(service.Endpoint)
	if err != nil {
		return nil, err
	}

	httpClient := http.Client{}
	req, err := http.NewRequest("GET", target.String(), nil)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Accept", acceptHeader)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned HTTP status %s", resp.Status)
	}

	dec := expfmt.NewDecoder(resp.Body, expfmt.ResponseFormat(resp.Header))

	var metricFamilies map[string]*dto.MetricFamily = make(map[string]*dto.MetricFamily)

	for {
		var d *dto.MetricFamily = &dto.MetricFamily{}

		if err = dec.Decode(d); err != nil {
			break
		}

		labels, _ := p.getLabels(serviceName)

		for _, metric := range d.Metric {
			for k, v := range labels {
				metric.Label = append(metric.Label, &dto.LabelPair{Name: proto.String(k), Value: proto.String(v)})
			}

			// if we passed in extra labels via query param, add those too
			if len(adhocLabels) > 0 {
				for name, value := range adhocLabels {
					metric.Label = append(metric.Label, &dto.LabelPair{Name: proto.String(name),
						Value: proto.String(value)})
				}
			}
		}
		metricFamilies[*d.Name] = d
	}
	return metricFamilies, err
}