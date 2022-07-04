package datadog

import (
	"bytes"
	"compress/zlib"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/DataDog/datadog-firehose-nozzle/internal/logs"
	"github.com/DataDog/datadog-firehose-nozzle/internal/metric"
	"github.com/cloudfoundry/gosteno"
)

type Formatter struct {
	log *gosteno.Logger
}

type FormatData struct {
	data     []byte
	nbrItems uint64
}

func (f Formatter) Format(prefix string, maxPostBytes uint32, data map[metric.MetricKey]metric.MetricValue) []FormatData {
	if len(data) == 0 {
		return nil
	}

	var result []FormatData
	compressedSeriesBytes, err := f.formatMetrics(prefix, data)
	if err != nil {
		f.log.Errorf("Error formatting metrics payload: %v", err)
		return result
	}

	if uint32(len(compressedSeriesBytes)) > maxPostBytes {
		if len(data) == 1 {
			for k, _ := range data {
				f.log.Warn(fmt.Sprintf("Warning dropping metric payload: %v", k.Name))
			}
			return nil
		}

		metricsA, metricsB := splitMetrics(data)

		result = append(result, f.Format(prefix, maxPostBytes, metricsA)...)
		result = append(result, f.Format(prefix, maxPostBytes, metricsB)...)

		return result
	}

	result = append(result, FormatData{data: compressedSeriesBytes, nbrItems: uint64(len(data))})
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

		_type := mVal.Type

		if _type == "" {
			_type = "gauge"
		}

		m := metric.Series{
			Metric: name,
			Points: points,
			Type:   _type,
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

func (f Formatter) FormatLogs(maxPostBytes uint32, data []logs.LogMessage) []FormatData {
	if len(data) == 0 {
		return nil
	}

	var result []FormatData
	compressedLogsBytes, err := f.formatLogs(data)
	if err != nil {
		f.log.Errorf("Error formatting logs payload: %v", err)
		return result
	}

	if uint32(len(compressedLogsBytes)) > maxPostBytes {
		if len(data) == 1 {
			f.log.Warn(fmt.Sprintf("Warning dropping logs payload for service: %v", data[0].Service))
			return nil
		}

		logsA, logsB := splitLogs(data)

		result = append(result, f.FormatLogs(maxPostBytes, logsA)...)
		result = append(result, f.FormatLogs(maxPostBytes, logsB)...)

		return result
	}

	result = append(result, FormatData{data: compressedLogsBytes, nbrItems: uint64(len(data))})
	return result
}

func (f Formatter) formatLogs(data []logs.LogMessage) ([]byte, error) {
	s := []logs.LogMessage{}

	for _, entry := range data {
		e := entry
		s = append(s, e)
	}

	encodedLogs, err := json.Marshal(s)
	if err != nil {
		return nil, fmt.Errorf("Error marshalling logs: %v", err)
	}
	compressedPayload, err := compress(encodedLogs)
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

func splitLogs(data []logs.LogMessage) (a, b []logs.LogMessage) {
	l := len(data)
	a = data[l/2:]
	b = data[:l/2]
	return a, b
}

func splitMetrics(data map[metric.MetricKey]metric.MetricValue) (a, b map[metric.MetricKey]metric.MetricValue) {
	a = make(map[metric.MetricKey]metric.MetricValue)
	b = make(map[metric.MetricKey]metric.MetricValue)

	for k, v := range data {
		if len(a) < len(data)/2 {
			a[k] = v
		} else {
			b[k] = v
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
