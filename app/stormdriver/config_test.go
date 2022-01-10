package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_ParseFile(t *testing.T) {
	tests := []struct {
		name    string
		input   []byte
		wantOut *configuration
	}{
		{
			"empty sets defaults",
			[]byte(``),
			&configuration{
				ListenPort:            defaultHTTPPort,
				DialTimeout:           defaultDialTimeout,
				ClientTimeout:         defaultClientTimeout,
				TLSHandshakeTimeout:   defaultTLSHandshakeTimeout,
				ResponseHeaderTimeout: defaultResponseHeaderTimeout,
				MaxIdleConnections:    defaultMaxIdleConns,
				Clouddrivers:          []clouddriverConfig{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual, err := loadConfiguration(tt.input)
			require.NoError(t, err)
			require.NotNil(t, actual)
			assert.Equal(t, tt.wantOut, actual)
		})
	}

}
