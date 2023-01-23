package helper

type FakeResponse struct {
	status int
	data   []byte
}

func NewFakeResponse(status int, data []byte) *FakeResponse {
	return &FakeResponse{
		status: status,
		data:   data,
	}
}
