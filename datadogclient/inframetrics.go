package datadogclient

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-firehose-nozzle/metrics"
	"github.com/DataDog/datadog-firehose-nozzle/utils"
	"github.com/cloudfoundry/sonde-go/events"
)

func (c *Client) ParseInfraMetric(envelope *events.Envelope) (metrics.MetricKey, metrics.MetricValue, error) {
	c.mLock.Lock()
	defer c.mLock.Unlock()

	if envelope.GetEventType() != events.Envelope_ValueMetric && envelope.GetEventType() != events.Envelope_CounterEvent {
		return metrics.MetricKey{}, metrics.MetricValue{}, fmt.Errorf("not an infra metric")
	}

	tags := parseTags(envelope)
	host := parseHost(envelope)

	key := metrics.MetricKey{
		EventType: envelope.GetEventType(),
		Name:      getName(envelope),
		TagsHash:  utils.HashTags(tags),
	}

	mVal := metrics.MetricValue{}
	value := getValue(envelope)

	mVal.Host = host
	mVal.Tags = tags
	mVal.Points = append(mVal.Points, metrics.Point{
		Timestamp: envelope.GetTimestamp() / int64(time.Second),
		Value:     value,
	})

	return key, mVal, nil
}

func getName(envelope *events.Envelope) string {
	switch envelope.GetEventType() {
	case events.Envelope_ValueMetric:
		return envelope.GetOrigin() + "." + envelope.GetValueMetric().GetName()
	case events.Envelope_CounterEvent:
		return envelope.GetOrigin() + "." + envelope.GetCounterEvent().GetName()
	default:
		panic("Unknown event type")
	}
}

func getValue(envelope *events.Envelope) float64 {
	switch envelope.GetEventType() {
	case events.Envelope_ValueMetric:
		return envelope.GetValueMetric().GetValue()
	case events.Envelope_CounterEvent:
		return float64(envelope.GetCounterEvent().GetTotal())
	default:
		panic("Unknown event type")
	}
}

func parseTags(envelope *events.Envelope) []string {
	tags := appendTagIfNotEmpty(nil, "deployment", envelope.GetDeployment())
	tags = appendTagIfNotEmpty(tags, "job", envelope.GetJob())
	tags = appendTagIfNotEmpty(tags, "index", envelope.GetIndex())
	tags = appendTagIfNotEmpty(tags, "ip", envelope.GetIp())
	tags = appendTagIfNotEmpty(tags, "origin", envelope.GetOrigin())
	tags = appendTagIfNotEmpty(tags, "name", envelope.GetOrigin())
	for tname, tvalue := range envelope.GetTags() {
		tags = appendTagIfNotEmpty(tags, tname, tvalue)
	}
	return tags
}

func parseHost(envelope *events.Envelope) string {
	if envelope.GetIndex() != "" {
		return envelope.GetIndex()
	} else if envelope.GetOrigin() != "" {
		return envelope.GetOrigin()
	}

	return ""
}

func appendTagIfNotEmpty(tags []string, key, value string) []string {
	if value != "" {
		tags = append(tags, fmt.Sprintf("%s:%s", key, value))
	}
	return tags
}
