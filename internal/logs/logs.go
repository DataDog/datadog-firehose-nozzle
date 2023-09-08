package logs

type LogMessage struct {
	Source   string `json:"ddsource"`
	Tags     string `json:"ddtags"`
	Hostname string `json:"hostname"`
	Message  string `json:"message"`
	Service  string `json:"service"`
}
