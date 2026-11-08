package handlers

import (
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

func TestPlayerOpenOnPublicNetwork_GH663(t *testing.T) {
	tests := []struct {
		name    string
		mode    string
		enabled bool
		want    bool
	}{
		{name: "public with player", mode: deploymentModePublicNetwork, enabled: true, want: true},
		{name: "public without player", mode: deploymentModePublicNetwork, enabled: false, want: false},
		{name: "private with player", mode: "private-network", enabled: true, want: false},
		{name: "unset with player", mode: "", enabled: true, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ds := datastore.NewDataStore(t.TempDir())
			_ = ds.Initialize()

			_, server := setupRouter("http://127.0.0.1:8000", ds)
			server.SetDeploymentMode(tt.mode)
			server.SetPlayerEnabled(tt.enabled)

			if got := server.playerOpenOnPublicNetwork(); got != tt.want {
				t.Errorf("playerOpenOnPublicNetwork() = %v, want %v", got, tt.want)
			}
		})
	}
}
