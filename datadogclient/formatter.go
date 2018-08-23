package datadogclient

import (
	"encoding/json"

	"github.com/cloudfoundry/gosteno"

	"github.com/DataDog/datadog-firehose-nozzle/metrics"
)

type Formatter struct {
	log *gosteno.Logger
}

func (f Formatter) Format(prefix string, maxPostBytes uint32, data map[metrics.MetricKey]metrics.MetricValue) [][]byte {
	if len(data) == 0 {
		return nil
	}

	var result [][]byte
	seriesBytes := f.formatMetrics(prefix, data)
	if uint32(len(seriesBytes)) > maxPostBytes && canSplit(data) {
		metricsA, metricsB := splitPoints(data)
		result = append(result, f.Format(prefix, maxPostBytes, metricsA)...)
		result = append(result, f.Format(prefix, maxPostBytes, metricsB)...)

		return result
	}

	result = append(result, seriesBytes)
	return result
}

func (f Formatter) formatMetrics(prefix string, data map[metrics.MetricKey]metrics.MetricValue) []byte {
	var s []metrics.Series
	for key, mVal := range data {
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
		f.log.Errorf("Error marshalling metrics: %v", err)
	}
	return encodedMetric
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
