package datadog

import (
	"bytes"
	"compress/zlib"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/DataDog/datadog-firehose-nozzle/internal/metric"
	"github.com/cloudfoundry/gosteno"
)

type Formatter struct {
	log *gosteno.Logger
}

func (f Formatter) Format(prefix string, maxPostBytes uint32, data map[metric.MetricKey]metric.MetricValue) [][]byte {
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

func (f Formatter) formatMetrics(prefix string, data map[metric.MetricKey]metric.MetricValue) ([]byte, error) {
	s := []metric.Series{}
	for key, mVal := range data {
		// dogate feature
		if strings.HasPrefix(key.Name, "bosh.healthmonitor") {
			prefix = ""
		}

		name := prefix + key.Name
		points := f.removeNANs(mVal.Points, name, mVal.Tags)

		m := metric.Series{
			Metric: name,
			Points: points,
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

func (f Formatter) removeNANs(points []metric.Point, metricName string, tags []string) []metric.Point {
	var sanitizedPoints []metric.Point
	for _, point := range points {
		if !math.IsNaN(point.Value) {
			sanitizedPoints = append(sanitizedPoints, point)
		} else {
			f.log.Errorf("Point has NAN value.  Dropping: %s %+v", metricName, tags)
		}
	}
	return sanitizedPoints

}

func canSplit(data map[metric.MetricKey]metric.MetricValue) bool {
	for _, v := range data {
		if len(v.Points) > 1 {
			return true
		}
	}

	return false
}

func splitPoints(data map[metric.MetricKey]metric.MetricValue) (a, b map[metric.MetricKey]metric.MetricValue) {
	a = make(map[metric.MetricKey]metric.MetricValue)
	b = make(map[metric.MetricKey]metric.MetricValue)
	for k, v := range data {
		split := len(v.Points) / 2
		if split == 0 {
			a[k] = metric.MetricValue{
				Tags:   v.Tags,
				Points: v.Points,
			}
			continue
		}

		a[k] = metric.MetricValue{
			Tags:   v.Tags,
			Points: v.Points[:split],
		}
		b[k] = metric.MetricValue{
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
