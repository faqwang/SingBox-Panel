package panel

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/hann0w0/singbox-panel/internal/domain/model"
)

func TestInboundSubscriptionOptions(t *testing.T) {
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
		`{"type":"socks","tag":"socks-in","listen_port":1080,"subscription_port":443,"node_name":"  Edge Premium  "}`)
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
	update(`{"node_name":"bad\nname"}`, http.StatusBadRequest)
	update(`{"listen_port":1081}`, http.StatusOK)

	checkNode := func(name string, port int) {
		t.Helper()
		for _, output := range []struct {
			target string
			want   []string
		}{
			{"links", []string{fmt.Sprintf("socks5://edge.example.com:%d#%s", port, url.PathEscape(name))}},
			{"sing-box", []string{fmt.Sprintf(`"server_port": %d`, port), fmt.Sprintf(`"tag": %q`, name)}},
			{"clash", []string{fmt.Sprintf("port: %d", port), "name: " + name}},
			{"shadowrocket", []string{fmt.Sprintf("port: %d", port), "name: " + name}},
			{"surge", []string{fmt.Sprintf("%s = socks5, edge.example.com, %d,", name, port)}},
		} {
			res := serveJSONRequest(t, router, http.MethodGet, "/sub/"+user.SubToken+"?target="+output.target, "")
			for _, want := range output.want {
				if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), want) {
					t.Errorf("%s subscription: %d %s, want %q", output.target, res.Code, res.Body, want)
				}
			}
		}
		res := serveJSONRequest(t, router, http.MethodGet, serverPath+"/node-formats", "")
		var exported struct{ Items []NodeFormatItem }
		if err := json.Unmarshal(res.Body.Bytes(), &exported); err != nil {
			t.Fatal(err)
		}
		if res.Code != http.StatusOK || len(exported.Items) != 1 || exported.Items[0].Name != name || exported.Items[0].Port != port {
			t.Errorf("node export: %d %s, want %q and port %d", res.Code, res.Body, name, port)
		}
	}
	checkNode("Edge Premium", 443)
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
	checkNode("Edge Premium", 1081)
	if result := update(`{"node_name":" "}`, http.StatusOK); strings.Contains(result, `"apply_state"`) {
		t.Fatalf("subscription-only edit staged a server config: %s", result)
	}
	checkNode("Edge", 1081)
	after, err := app.orch.BuildServerConfig(&server)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("changing subscription options modified the server config: %s", after)
	}
}
