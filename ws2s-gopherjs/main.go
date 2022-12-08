//go:build js
// +build js

package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/gopherjs/gopherjs/js"
	"time"
)

func Log(format string, args ...interface{}) {
	js.Global.Get("console").Call("log", fmt.Sprintf(format, args...))
}

type OptionalParams struct {
	Host                  string `json:"host,omitempty"`
	Port                  int    `json:"port,omitempty"`
	SSHHost               string `json:"ssh_host,omitempty"`
	SSHPort               int    `json:"ssh_port,omitempty"`
	SSHUsername           string `json:"ssh_username,omitempty"`
	SSHPassword           string `json:"ssh_password,omitempty"`
	SSHPrivateKey         string `json:"ssh_private_key,omitempty"`
	SSHPrivateKeyPassword string `json:"ssh_private_key_password,omitempty"`
}

type wsCommand struct {
	OptionalParams
	Command   string `json:"command"`
	PlainText bool   `json:"-"`
	Data      string `json:"data,omitempty"`
}

func (wsC *wsCommand) AsText() *wsCommand {
	wsC.PlainText = true
	return wsC
}

func (wsC *wsCommand) String() string {

	newInstance := *wsC
	if newInstance.PlainText || len(newInstance.Data) == 0 {
		textRaw, _ := json.Marshal(wsC)
		return string(textRaw)
	}
	newInstance.Data = base64.StdEncoding.EncodeToString([]byte(newInstance.Data))
	newInstance.PlainText = true
	return newInstance.String()
}

type connection struct {
	id             int
	ws2sServerUrl  string
	host           string
	port           int
	closed         bool
	doneChan       chan bool
	recvChan       chan []byte
	openChan       chan bool
	socket         *js.Object
	receiving      bool
	receivedBase64 []string

	onConnectHandler func()
	onErrorHandler   func(msg string)
}

func (c *connection) Connect() error {
	c.doneChan = make(chan bool)
	c.recvChan = make(chan []byte)
	c.openChan = make(chan bool)
	c.receivedBase64 = make([]string, 0)
	c.socket = js.Global.Get("WebSocket").New(c.ws2sServerUrl)
	if c.socket == nil {
		c.closed = true
		return fmt.Errorf("socket was not created")
	}

	c.socket.Set("onmessage", c.onMessage)
	c.socket.Set("onopen", c.onOpen)
	c.socket.Set("onclose", c.onClose)
	c.socket.Set("onerror", c.onError)

	if c.onConnectHandler != nil && c.onErrorHandler != nil {
		Log("running tailConnect in goroutine")
		go c.tailConnect()
		return nil
	}
	Log("running tailConnect directly")
	c.tailConnect()
	return nil
}

func (c *connection) tailConnect() {
	<-c.openChan
	if c.onConnectHandler != nil {
		go c.onConnectHandler()
	}
	c.send(wsCommand{
		OptionalParams: OptionalParams{
			Host: c.host,
			Port: c.port,
		},
		Command: "connect",
	})
	go func() {
		for {
			select {
			case <-c.doneChan:
				{
					return
				}
			case <-time.After(5 * time.Second):
				{
					c.send(wsCommand{
						Command: "ping",
					})
				}
			}
		}
	}()
}

func (c *connection) onMessage(event *js.Object) {
	go c.onMessageBackground(event)
}

func (c *connection) onMessageBackground(event *js.Object) {
	Log("new msg %v", event)
	response := js.Global.Get("JSON").Call("parse", event.Get("data").String())
	code := response.Get("code").Int()
	message := response.Get("message").String()
	Log("Code: %d Message: %s", code, message)
	if code < 0 {
		data := response.Get("data")
		Log("Data: %v(%d elements)", data, data.Length())
		//if data.Length() > 0 {
		//	Log("array of data")
		//	for idx := 0; idx < data.Length(); idx++ {
		//		c.receivedBase64 = append(c.receivedBase64, data.Index(idx).String())
		//	}
		//} else {
		//	Log("only 1 element")
		//	c.receivedBase64 = append(c.receivedBase64, data.String())
		//}
		c.receivedBase64 = append(c.receivedBase64, data.String())
		if !c.receiving {
			c.receiving = true

			for len(c.receivedBase64) > 0 {
				chunk := c.receivedBase64[0]
				c.receivedBase64 = c.receivedBase64[1:]

				decoded, err := base64.StdEncoding.DecodeString(chunk)
				if err != nil {
					Log("error while base64 decode chunk %s: %s", chunk, err)
					c.onError(&js.Object{})
				}

				c.recvChan <- decoded
			}

			c.receiving = false
		}
		return
	}
	if code == 0 {
		if message == "connect done" {
			c.onOpen()
		}
		if message == "close done" {
			c.onClose()
		}
		return
	}
	if code == 5 || code == 3 {
		_ = c.Close()
	}
	c.onError(response)
}

func (c *connection) onOpen() {
	Log("opened")
	c.openChan <- true
}

func (c *connection) onClose() {
	Log("closed")
	_ = c.Close()
}

func (c *connection) onError(err *js.Object) {
	Log("error occurred: %v", err)
	_ = c.Close()
}

func (c *connection) Read() ([]byte, error) {
	return <-c.recvChan, nil
}

func (c *connection) Write(data []byte) (int, error) {
	c.send(wsCommand{
		Command: "sendb",
		Data:    string(data),
	})
	return 0, nil
}

func (c *connection) Close() error {
	if !c.closed {
		close(c.doneChan)
		c.closed = true
		c.close()
		c.socket.Call("close")
	}
	return nil
}

func (c *connection) close() {
	c.send(wsCommand{
		Command: "close",
	})
}

func (c *connection) send(command wsCommand) {
	c.socket.Call("send", command.String())
}

var connections map[int]*connection
var connId int

func NewConnection(
	ws2sHost string, host string, port int,
	onConnect func(),
	onError func(msg string),
) (int, error) {

	connectionID := connId
	connId++
	conn := connection{
		id:               connectionID,
		ws2sServerUrl:    ws2sHost,
		host:             host,
		port:             port,
		onConnectHandler: onConnect,
		onErrorHandler:   onError,
	}
	connections[connectionID] = &conn
	return connectionID, conn.Connect()
}

func ReadConnection(id int, onReadCallback func(data []byte, err string)) ([]byte, error) {
	Log("entered ReadConnection %d", id)
	conn, ok := connections[id]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	if onReadCallback != nil {
		go func() {
			data, err := conn.Read()
			if err != nil {
				Log("ReadConnection %d error: %v", id, err)
				onReadCallback(nil, err.Error())
			} else {
				onReadCallback(data, "")
			}
		}()
		return nil, nil
	}
	return conn.Read()
}

func WriteConnection(id int, data []byte, onWriteCallback func(n int, err string)) (int, error) {
	Log("entered WriteConnection %d", id)
	conn, ok := connections[id]
	if !ok {
		return 0, fmt.Errorf("not found")
	}
	if onWriteCallback != nil {
		go func() {
			n, err := conn.Write(data)
			if err != nil {
				Log("WriteConnection %d error: %v", id, err)
				onWriteCallback(n, err.Error())
			} else {
				onWriteCallback(n, "")
			}
		}()
		return 0, nil
	}
	return conn.Write(data)
}

func CloseConnection(id int, onCloseCallback func()) error {
	Log("entered CloseConnection %d", id)
	conn, ok := connections[id]
	if !ok {
		return fmt.Errorf("not found")
	}
	if onCloseCallback != nil {
		go func() {
			err := conn.Close()
			if err != nil {
				Log("close %d error: %v", id, err)
			}
			onCloseCallback()
		}()
		return nil
	} else {
		return conn.Close()
	}
}

func init() {
	connections = make(map[int]*connection)

	js.Global.Set("hash_it", hashIt)
	js.Global.Set("print_obj", func(obj *js.Object) {
		js.Global.Get("console").Call("log", fmt.Sprintf("obj size: %d", obj.Length()))
		Log("obj.a: %s", obj.Get("a"))
		Log("obj.b: %d", obj.Get("b").Int())
		Log("calling Func")
		obj.Call("Func", "aaa", 5)
	})
	js.Global.Set("WS2S_connect", func(obj *js.Object) int {
		ws2sHost := obj.Get("ws2s_host").String()
		host := obj.Get("host").String()
		port := obj.Get("port").Int()
		onConnectField := obj.Get("on_connect")
		var onConnect func()
		if onConnectField != nil {
			onConnect = func() {
				onConnectField.Invoke()
			}
		}
		onErrorField := obj.Get("on_error")
		var onError func(msg string)
		if onErrorField != nil {
			onError = func(msg string) {
				onErrorField.Invoke(msg)
			}
		}
		id, err := NewConnection(ws2sHost, host, port, onConnect, onError)
		if err != nil {
			Log("connect error: %v", err)
			return -1
		}
		return int(id)
	})
	js.Global.Set("WS2S_write", func(obj *js.Object) (int, error) {
		id := obj.Get("connection").Int()
		data := obj.Get("data").String()
		onWriteCallbackField := obj.Get("callback")
		var onWriteCallback func(n int, err string)
		if onWriteCallbackField != nil {
			onWriteCallback = func(n int, err string) {
				onWriteCallbackField.Invoke(n, err)
			}
		}
		return WriteConnection(id, []byte(data), onWriteCallback)
	})
	js.Global.Set("WS2S_read", func(obj *js.Object) ([]byte, error) {
		id := obj.Get("connection").Int()
		onReadCallbackField := obj.Get("callback")
		var onReadCallback func(data []byte, err string)
		if onReadCallbackField != nil {
			onReadCallback = func(data []byte, err string) {
				onReadCallbackField.Invoke(data, err)
			}
		}
		return ReadConnection(id, onReadCallback)
	})
	js.Global.Set("WS2S_close", func(obj *js.Object) error {
		id := obj.Get("connection").Int()
		onCloseCallbackField := obj.Get("callback")
		var onCloseCallback func()
		if onCloseCallbackField != nil {
			onCloseCallback = func() {
				onCloseCallbackField.Invoke()
			}
		}
		return CloseConnection(id, onCloseCallback)
	})

	js.Global.Set("WS2S_echo_test_background", func() {
		go func() {
			conn, err := NewConnection("ws://127.0.0.1:3613", "127.0.0.1", 9001, nil, nil)
			if err != nil {
				Log("err connection: %v", err)
				return
			}
			Log("new test conn: %d", conn)
			_, err = WriteConnection(conn, []byte("some imput"), nil)
			if err != nil {
				Log("err write: %v", err)
				return
			}

			data, err := ReadConnection(conn, nil)
			if err != nil {
				Log("err read: %v", err)
				return
			}
			Log("input data: %s", data)
			err = CloseConnection(conn, nil)
			if err != nil {
				Log("err close: %v", err)
				return
			}
			Log("SUCCESS !!!")
		}()
	})
}

func main() {

}

func hashIt(s string) []byte {
	h := sha256.New()
	h.Write([]byte(s))
	return h.Sum(nil)
}
