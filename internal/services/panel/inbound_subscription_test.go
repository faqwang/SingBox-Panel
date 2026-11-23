package panel

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/hann0w0/singbox-panel/internal/domain/model"
)

func TestInboundSubscriptionPort(t *testing.T) {
	db := testDB(t)
	server := model.Server{Name: "Edge", Address: "edge.example.com", AgentToken: "subscription-port-agent"}
	mustCreate(t, db, &server)
	user := model.User{Email: "subscriber", Enabled: true, ServerIDs: []uint{server.ID}, SubToken: "subscription-port-user"}
	mustCreate(t, db, &user)
	app := &App{db: db, orch: NewOrchestrator(db, nil)}
	router := gin.New()
	router.POST("/servers/:id/inbounds", app.createInbound)
	router.PUT("/servers/:id/inbounds/:inboundID", app.updateInbound)
	router.GET("/servers/:id/node-formats", app.serverNodeFormats)
	router.GET("/sub/:token", app.handleSubscription)
	serverPath := fmt.Sprintf("/servers/%d", server.ID)
	res := serveJSONRequest(t, router, http.MethodPost, serverPath+"/inbounds",
		`{"type":"socks","tag":"socks-in","listen_port":1080,"subscription_port":443}`)
	if res.Code != http.StatusOK {
		t.Fatalf("create inbound: %d %s", res.Code, res.Body)
	}
	var created struct{ Inbound model.Inbound }
	if err := json.Unmarshal(res.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("%s/inbounds/%d", serverPath, created.Inbound.ID)
	update := func(body string, status int) string {
		t.Helper()
		res := serveJSONRequest(t, router, http.MethodPut, path, body)
		if res.Code != status {
			t.Fatalf("update %s: %d %s", body, res.Code, res.Body)
		}
		return res.Body.String()
	}
	update(`{"subscription_port":65536}`, http.StatusBadRequest)
	update(`{"listen_port":1081}`, http.StatusOK)

	checkPort := func(port int) {
		t.Helper()
		for _, output := range []struct{ target, want string }{
			{"links", fmt.Sprintf("socks5://edge.example.com:%d#Edge", port)},
			{"sing-box", fmt.Sprintf(`"server_port": %d`, port)},
			{"clash", fmt.Sprintf("port: %d", port)},
			{"shadowrocket", fmt.Sprintf("port: %d", port)},
			{"surge", fmt.Sprintf("Edge = socks5, edge.example.com, %d,", port)},
		} {
			res := serveJSONRequest(t, router, http.MethodGet, "/sub/"+user.SubToken+"?target="+output.target, "")
			if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), output.want) {
				t.Errorf("%s subscription: %d %s, want %q", output.target, res.Code, res.Body, output.want)
			}
		}
		res := serveJSONRequest(t, router, http.MethodGet, serverPath+"/node-formats", "")
		var exported struct{ Items []NodeFormatItem }
		if err := json.Unmarshal(res.Body.Bytes(), &exported); err != nil {
			t.Fatal(err)
		}
		if res.Code != http.StatusOK || len(exported.Items) != 1 || exported.Items[0].Port != port {
			t.Errorf("node export: %d %s, want port %d", res.Code, res.Body, port)
		}
	}
	checkPort(443)
	before, err := app.orch.BuildServerConfig(&server)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(before), `"listen_port": 1081`) {
		t.Fatalf("server config did not use the listen port: %s", before)
	}
	if result := update(`{"subscription_port":0}`, http.StatusOK); strings.Contains(result, `"apply_state"`) {
		t.Fatalf("subscription-only edit staged a server config: %s", result)
	}
	checkPort(1081)
	after, err := app.orch.BuildServerConfig(&server)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("changing subscription port modified the server config: %s", after)
	}
}
