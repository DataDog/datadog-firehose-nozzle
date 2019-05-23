package datadogclient

import (
	"bytes"
	"compress/zlib"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-firehose-nozzle/metrics"
	"github.com/cloudfoundry/gosteno"
)

type Formatter struct {
	log *gosteno.Logger
}

func (f Formatter) Format(prefix string, maxPostBytes uint32, data map[metrics.MetricKey]metrics.MetricValue) [][]byte {
	if len(data) == 0 {
		return nil
	}

	var result [][]byte
	compressedSeriesBytes, err := f.formatMetrics(prefix, data)
	if err != nil {
		f.log.Errorf("Error formatting metrics payload: %v", err)
		return result
	}
	if uint32(len(compressedSeriesBytes)) > maxPostBytes && canSplit(data) {
		metricsA, metricsB := splitPoints(data)
		result = append(result, f.Format(prefix, maxPostBytes, metricsA)...)
		result = append(result, f.Format(prefix, maxPostBytes, metricsB)...)

		return result
	}

	result = append(result, compressedSeriesBytes)
	return result
}

func (f Formatter) formatMetrics(prefix string, data map[metrics.MetricKey]metrics.MetricValue) ([]byte, error) {
	s := []metrics.Series{}
	for key, mVal := range data {
		// dogate feature
		if strings.HasPrefix(key.Name, "bosh.healthmonitor") {
			prefix = ""
		}
		m := metrics.Series{
			Metric: prefix + key.Name,
			Points: mVal.Points,
			Type:   "gauge",
			Tags:   mVal.Tags,
			Host:   mVal.Host,
		}
		s = append(s, m)
	}

	encodedMetric, err := json.Marshal(Payload{Series: s})
	if err != nil {
		return nil, fmt.Errorf("Error marshalling metrics: %v", err)
	}
	compressedPayload, err := compress(encodedMetric)
	if err != nil {
		return nil, fmt.Errorf("Error compressing payload: %v", err)
	}
	return compressedPayload, nil
}

func canSplit(data map[metrics.MetricKey]metrics.MetricValue) bool {
	for _, v := range data {
		if len(v.Points) > 1 {
			return true
		}
	}

	return false
}

func splitPoints(data map[metrics.MetricKey]metrics.MetricValue) (a, b map[metrics.MetricKey]metrics.MetricValue) {
	a = make(map[metrics.MetricKey]metrics.MetricValue)
	b = make(map[metrics.MetricKey]metrics.MetricValue)
	for k, v := range data {
		split := len(v.Points) / 2
		if split == 0 {
			a[k] = metrics.MetricValue{
				Tags:   v.Tags,
				Points: v.Points,
			}
			continue
		}

		a[k] = metrics.MetricValue{
			Tags:   v.Tags,
			Points: v.Points[:split],
		}
		b[k] = metrics.MetricValue{
			Tags:   v.Tags,
			Points: v.Points[split:],
		}
	}
	return a, b
}

// Compress will compress the data with zlib
func compress(src []byte) ([]byte, error) {
	var b bytes.Buffer
	w := zlib.NewWriter(&b)
	_, err := w.Write(src)
	if err != nil {
		return nil, err
	}
	err = w.Close()
	if err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
