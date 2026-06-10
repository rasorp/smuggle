package wireguard

import (
	"testing"

	"github.com/shoenig/test/must"
	"go.uber.org/zap"
)

func TestOperatorConfig_Validate(t *testing.T) {
	tests := []struct {
		name          string
		inputConfig   *OperatorConfig
		expectedError bool
	}{
		{
			name: "valid listen port",
			inputConfig: &OperatorConfig{
				ListenPort: 51820,
			},
			expectedError: false,
		},
		{
			name: "negative listen port",
			inputConfig: &OperatorConfig{
				ListenPort: -1,
			},
			expectedError: true,
		},
		{
			name: "listen port above 65535",
			inputConfig: &OperatorConfig{
				ListenPort: 70000,
			},
			expectedError: true,
		},
		{
			name: "valid persistent keepalive",
			inputConfig: &OperatorConfig{
				PersistentKeepalive: 25,
			},
			expectedError: false,
		},
		{
			name: "zero persistent keepalive is valid",
			inputConfig: &OperatorConfig{
				PersistentKeepalive: 0,
			},
			expectedError: false,
		},
		{
			name: "negative persistent keepalive",
			inputConfig: &OperatorConfig{
				PersistentKeepalive: -1,
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actualError := tt.inputConfig.Validate()
			if tt.expectedError {
				must.Error(t, actualError)
			} else {
				must.NoError(t, actualError)
			}
		})
	}
}

func Test_peerConfig_loggingPairs(t *testing.T) {

	testConfig := &PeerConfig{
		ListenPort: 51820,
		PublicKey:  "test-public-key-base64==",
	}

	expectedPairs := []zap.Field{
		zap.Int("listen_port", 51820),
		zap.String("public_key", "test-public-key-base64=="),
	}

	must.SliceContainsAll(t, expectedPairs, testConfig.loggingPairs())
}
