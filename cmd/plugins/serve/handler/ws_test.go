package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cangyunye/go-owl/cmd/plugins/serve/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

func TestWSHub_BroadcastTaskUpdate(t *testing.T) {
	hub := NewWSHub()

	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		require.NoError(t, err)
		hub.Subscribe(r.Context(), conn)
		<-r.Context().Done()
	}))
	defer s.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, s.URL, nil)
	require.NoError(t, err)
	defer conn.CloseNow()

	time.Sleep(50 * time.Millisecond)

	type fakeTask struct {
		ID      string `json:"id"`
		Status  string `json:"status"`
		Command string `json:"command"`
		Output  string `json:"output,omitempty"`
	}
	task := &fakeTask{ID: "task-1", Status: "running", Command: "ls -la"}
	hub.BroadcastTaskUpdate(task)

	var msg struct {
		Type string `json:"type"`
		Data fakeTask `json:"data"`
	}
	err = wsjson.Read(ctx, conn, &msg)
	require.NoError(t, err)
	require.Equal(t, "task_update", msg.Type)
	require.Equal(t, "task-1", msg.Data.ID)
	require.Equal(t, "running", msg.Data.Status)
}

func TestWSHub_MultipleClients(t *testing.T) {
	hub := NewWSHub()

	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		require.NoError(t, err)
		hub.Subscribe(r.Context(), conn)
		<-r.Context().Done()
	}))
	defer s.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn1, _, err := websocket.Dial(ctx, s.URL, nil)
	require.NoError(t, err)
	defer conn1.CloseNow()

	conn2, _, err := websocket.Dial(ctx, s.URL, nil)
	require.NoError(t, err)
	defer conn2.CloseNow()

	time.Sleep(50 * time.Millisecond)

	type fakeTask struct {
		ID string `json:"id"`
	}
	task := &fakeTask{ID: "task-2"}
	hub.BroadcastTaskUpdate(task)

	for i, conn := range []*websocket.Conn{conn1, conn2} {
		var msg struct {
			Type string   `json:"type"`
			Data fakeTask `json:"data"`
		}
		err := wsjson.Read(ctx, conn, &msg)
		require.NoError(t, err)
		require.Equal(t, "task_update", msg.Type)
		require.Equal(t, "task-2", msg.Data.ID, "client %d", i+1)
	}
}

func TestWSHandler_NoToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	hub := NewWSHub()
	auth := NewAuthHandler(nil, nil)
	r.GET("/ws", hub.WsHandler(auth))

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/ws", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, 400, w.Code)
}

func TestWSHandler_InvalidToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	hub := NewWSHub()
	as := service.NewAuthService("test-secret-32byte-long-string!!")
	auth := NewAuthHandler(nil, as)
	r.GET("/ws", hub.WsHandler(auth))

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/ws?token=badtoken", nil)
	r.ServeHTTP(w, req)
	require.Equal(t, 401, w.Code)
}

func TestWSHandler_UpgradeAndBroadcast(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	hub := NewWSHub()
	as := service.NewAuthService("test-secret-32byte-long-string!!")
	auth := NewAuthHandler(nil, as)
	token, err := as.GenerateToken("admin", "admin")
	require.NoError(t, err)
	r.GET("/ws", hub.WsHandler(auth))

	s := httptest.NewServer(r)
	defer s.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, s.URL+"/ws?token="+token, nil)
	require.NoError(t, err)
	defer conn.CloseNow()

	time.Sleep(50 * time.Millisecond)

	type fakeTask struct {
		ID string `json:"id"`
	}
	hub.BroadcastTaskUpdate(&fakeTask{ID: "ws-task"})

	var msg struct {
		Type string   `json:"type"`
		Data fakeTask `json:"data"`
	}
	err = wsjson.Read(ctx, conn, &msg)
	require.NoError(t, err)
	require.Equal(t, "task_update", msg.Type)
	require.Equal(t, "ws-task", msg.Data.ID)
}
