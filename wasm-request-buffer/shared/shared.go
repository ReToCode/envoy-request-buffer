package shared

import (
	"strings"
)

const (
	VMID                    = "request-buffer"
	ScaledToZeroClustersKey = "scaled_to_zero_clusters_key"

	splitter = "~"
)

type RequestContext struct {
	Authority     string
	HttpContextID uint32
}

// Note:
// As tinygo does not support serialization well just yet, we use our own string serialization for now
// https://github.com/tinygo-org/tinygo/issues/447

func EncodeSharedData(data []string) []byte {
	return []byte(strings.Join(data, splitter))
}

func DecodeSharedData(data []byte) []string {
	str := string(data)
	return strings.Split(str, splitter)
}
