package metric

import (
	"errors"
	"fmt"
)

const (
	GAUGE = "gauge"
	COUNT = "count"
	RATE  = "rate"
)

type Point struct {
	Timestamp int64
	Value     float64
}

func (p Point) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`[%d, %f]`, p.Timestamp, p.Value)), nil
}

func (p *Point) UnmarshalJSON(in []byte) error {
	var timestamp int64
	var value float64

	parsed, err := fmt.Sscanf(string(in), `[%d,%f]`, &timestamp, &value)
	if err != nil {
		return err
	}
	if parsed != 2 {
		return errors.New("expected two parsed values")
	}

	p.Timestamp = timestamp
	p.Value = value

	return nil
}

type MetricKey struct {
	Name     string
	TagsHash string
}

type MetricValue struct {
	Tags   []string
	Points []Point
	Host   string
	Type   string
}

type MetricPackage struct {
	MetricKey   *MetricKey
	MetricValue *MetricValue
}

type MetricsMap map[MetricKey]MetricValue

func (m MetricsMap) Add(key MetricKey, newVal MetricValue) {
	value, exists := m[key]
	if exists {
		value.Points = append(value.Points, newVal.Points...)
	} else {
		value = newVal
	}
	m[key] = value
}

type Series struct {
	Metric string   `json:"metric"`
	Points []Point  `json:"points"`
	Type   string   `json:"type"`
	Host   string   `json:"host,omitempty"`
	Tags   []string `json:"tags,omitempty"`
}
