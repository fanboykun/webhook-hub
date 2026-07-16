package teams

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fanboykun/webhook-hub/internal/config"
	"github.com/fanboykun/webhook-hub/internal/domain"
	"github.com/fanboykun/webhook-hub/internal/runtimeconfig"
)

func TestSendIncludesTeamsErrorBody(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`webhook disabled`))
	}))
	defer api.Close()

	sender := New(runtimeconfig.NewDestinationRegistry(map[string]config.DestinationConfig{
		"teams-oncall": {
			Type:        domain.DestinationTeams,
			ResolvedURL: api.URL,
		},
	}))

	_, err := sender.Send(context.Background(), "teams-oncall", domain.RenderedMessage{
		ContentType: "application/json",
		Body:        []byte(`{"text":"hello"}`),
	})
	if err == nil {
		t.Fatal("expected teams upstream rejection")
	}
	if got := err.Error(); got == "" || got == "teams returned 400" {
		t.Fatalf("expected response body in error, got %q", got)
	}
}
