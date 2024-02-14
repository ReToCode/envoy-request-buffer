package shared

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	VMID                    = "request-buffer"
	ScaledToZeroClustersKey = "scaled_to_zero_clusters_key"
	HTTPContextQueueName    = "http_context_queue"

	splitter                    = "~"
	failedToParseRequestContext = "failed to parse Request-Context: %s"
)

type RequestContext struct {
	Authority     string
	HttpContextID uint32
}

// Note:
// As tinygo does not support serialization well just yet, we use our own string serialization for now
// https://github.com/tinygo-org/tinygo/issues/447

func EncodeRequestContext(data RequestContext) []byte {
	var str string
	str += data.Authority + splitter + fmt.Sprint(data.HttpContextID)
	return []byte(str)
}

func DecodeRequestContext(data []byte) (*RequestContext, error) {
	str := string(data)

	if !strings.Contains(str, splitter) {
		return nil, fmt.Errorf(failedToParseRequestContext, str)
	}

	parts := strings.Split(str, splitter)

	if len(parts) != 2 {
		return nil, fmt.Errorf(failedToParseRequestContext, str)
	}

	parsed, err := strconv.ParseUint(parts[1], 10, 32)
	if err != nil {
		return nil, fmt.Errorf(failedToParseRequestContext, str)
	}

	return &RequestContext{
		Authority:     parts[0],
		HttpContextID: uint32(parsed),
	}, nil
}

func EncodeSharedData(data []string) []byte {
	return []byte(strings.Join(data, splitter))
}

func DecodeSharedData(data []byte) []string {
	str := string(data)
	return strings.Split(str, splitter)
}
